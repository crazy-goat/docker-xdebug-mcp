package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bp struct {
	file string // container path
	line int
	id   string // assigned by the engine on apply
}

type session struct {
	mu      sync.Mutex
	conn    net.Conn
	r       *bufio.Reader
	tx      int
	state   string // "no session" | "started" | "break" | "stopping"
	file    string // current location, host path
	line    int
	pending []bp
	ready   chan struct{} // closed on each adopt; lets ListenWait/DoRequest await a connection

	localRoot  string
	dockerRoot string
}

func newSession(localRoot, dockerRoot string) *session {
	return &session{
		state:      "no session",
		ready:      make(chan struct{}),
		localRoot:  strings.TrimRight(localRoot, "/"),
		dockerRoot: strings.TrimRight(dockerRoot, "/"),
	}
}

func (s *session) listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("DBGp listener on %s (local=%s docker=%s)", addr, s.localRoot, s.dockerRoot)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("accept: %v", err)
				continue
			}
			s.adopt(conn)
		}
	}()
	return nil
}

// adopt takes over a freshly accepted engine connection: reads <init>, sets
// features, applies pending breakpoints, and wakes any ListenWait/DoRequest.
func (s *session) adopt(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.conn = conn
	s.r = bufio.NewReader(conn)
	s.tx = 0
	s.file, s.line = "", 0

	initXML, err := s.readPacket()
	if err != nil {
		s.state = "no session"
		log.Printf("read init: %v", err)
		return
	}
	var ir struct {
		Fileuri string `xml:"fileuri,attr"`
	}
	unmarshal(initXML, &ir)
	s.state = "started"
	log.Printf("session started: %s", s.toHost(ir.Fileuri))

	s.rawLocked("feature_set", "-n max_depth -v 3")
	s.rawLocked("feature_set", "-n max_children -v 100")
	s.rawLocked("feature_set", "-n max_data -v 4096")
	for i := range s.pending {
		if r, _, err := s.rawLocked("breakpoint_set", fmt.Sprintf("-t line -f %s -n %d", fileURI(s.pending[i].file), s.pending[i].line)); err == nil && r != nil {
			s.pending[i].id = r.ID
		}
	}

	close(s.ready)
	s.ready = make(chan struct{})
}

// --- wire protocol ----------------------------------------------------------

// readPacket reads one length-prefixed, NUL-terminated DBGp packet: LEN\0XML\0
func (s *session) readPacket() (string, error) {
	lenStr, err := s.r.ReadString(0)
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(strings.TrimRight(lenStr, "\x00"))
	if err != nil {
		return "", fmt.Errorf("bad length %q: %w", lenStr, err)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(s.r, buf); err != nil {
		return "", err
	}
	s.r.ReadByte() // trailing NUL
	return string(buf), nil
}

// rawLocked sends one command and returns the parsed response. Caller holds mu.
func (s *session) rawLocked(name, args string) (*xResp, string, error) {
	if s.conn == nil {
		return nil, "", fmt.Errorf("no active session")
	}
	s.tx++
	line := name + " -i " + strconv.Itoa(s.tx)
	if args != "" {
		line += " " + args
	}
	if _, err := s.conn.Write([]byte(line + "\x00")); err != nil {
		s.state = "no session"
		return nil, "", err
	}
	xmlStr, err := s.readPacket()
	if err != nil {
		s.state = "no session"
		return nil, "", err
	}
	var r xResp
	unmarshal(xmlStr, &r)
	if r.Status != "" {
		s.state = r.Status
	}
	if r.Message != nil && r.Message.Filename != "" {
		s.file, s.line = s.toHost(r.Message.Filename), r.Message.Lineno
	}
	return &r, xmlStr, nil
}

// cmd is the locking wrapper used by public methods.
func (s *session) cmd(name, args string) (*xResp, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rawLocked(name, args)
}

// --- path translation -------------------------------------------------------

// toContainer maps a host (absolute or project-relative) path to the container path.
func (s *session) toContainer(p string) string {
	switch {
	case strings.HasPrefix(p, s.dockerRoot):
		return p
	case strings.HasPrefix(p, s.localRoot):
		return s.dockerRoot + p[len(s.localRoot):]
	case strings.HasPrefix(p, "/"):
		return p // some other absolute path; pass through
	default:
		return s.dockerRoot + "/" + strings.TrimLeft(p, "/")
	}
}

// toHost maps a container fileuri/path back to a host path for display.
func (s *session) toHost(fileuri string) string {
	p := strings.TrimPrefix(fileuri, "file://")
	if strings.HasPrefix(p, s.dockerRoot) {
		return s.localRoot + p[len(s.dockerRoot):]
	}
	return p
}

func fileURI(containerPath string) string { return "file://" + containerPath }

// --- public command methods (used by both MCP and HTTP front-ends) ----------

func (s *session) location() string {
	if s.file == "" {
		return "-"
	}
	return fmt.Sprintf("%s:%d", s.file, s.line)
}

func (s *session) Status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("state=%s\nlocation=%s\nbreakpoints=%d", s.state, s.location(), len(s.pending))
}

func (s *session) SetBreakpoint(file string, line int) (string, error) {
	if file == "" || line <= 0 {
		return "", fmt.Errorf("file and line>0 required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cpath := s.toContainer(file)
	b := bp{file: cpath, line: line}
	if s.conn != nil && (s.state == "started" || s.state == "break") {
		r, _, err := s.rawLocked("breakpoint_set", fmt.Sprintf("-t line -f %s -n %d", fileURI(cpath), line))
		if err != nil {
			return "", err
		}
		b.id = r.ID
		s.pending = append(s.pending, b)
		return fmt.Sprintf("breakpoint set id=%s %s:%d", b.id, cpath, line), nil
	}
	s.pending = append(s.pending, b)
	return fmt.Sprintf("breakpoint queued %s:%d (applied on next session)", cpath, line), nil
}

func (s *session) BreakpointList() (string, error) {
	r, _, err := s.cmd("breakpoint_list", "")
	if err != nil {
		return "", err
	}
	if len(r.Breakpoints) == 0 {
		s.mu.Lock()
		defer s.mu.Unlock()
		var b strings.Builder
		for _, p := range s.pending {
			fmt.Fprintf(&b, "queued %s:%d\n", s.toHost(p.file), p.line)
		}
		if b.Len() == 0 {
			return "(none)", nil
		}
		return b.String(), nil
	}
	var b strings.Builder
	for _, e := range r.Breakpoints {
		fmt.Fprintf(&b, "id=%s %s %s:%d\n", e.ID, e.State, s.toHost(e.Filename), e.Lineno)
	}
	return b.String(), nil
}

func (s *session) BreakpointRemove(id string) (string, error) {
	s.mu.Lock()
	for i, p := range s.pending {
		if p.id == id {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	if _, _, err := s.cmd("breakpoint_remove", "-d "+id); err != nil {
		return "", err
	}
	return "removed " + id, nil
}

// step runs run/step_into/step_over/step_out/break and reports the new location.
func (s *session) step(cmd string) (string, error) {
	r, _, err := s.cmd(cmd, "")
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := fmt.Sprintf("state=%s reason=%s\nlocation=%s", r.Status, r.Reason, s.location())
	if r.Status == "stopping" {
		out += "\n(script finished)"
	}
	return out, nil
}

func (s *session) Stack() (string, error) {
	r, _, err := s.cmd("stack_get", "")
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, st := range r.Stacks {
		fmt.Fprintf(&b, "#%d %s  %s:%d\n", st.Level, st.Where, s.toHost(st.Filename), st.Lineno)
	}
	if b.Len() == 0 {
		return "(no stack — not paused?)", nil
	}
	return b.String(), nil
}

func (s *session) Context(depth int) (string, error) {
	r, _, err := s.cmd("context_get", "-d "+strconv.Itoa(depth))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range r.Props {
		fmt.Fprintf(&b, "%s (%s) = %s\n", p.Name, p.Type, summarize(p))
	}
	if b.Len() == 0 {
		return "(no variables)", nil
	}
	return b.String(), nil
}

func (s *session) Eval(expr string) (string, error) {
	enc := base64.StdEncoding.EncodeToString([]byte(expr))
	r, _, err := s.cmd("eval", "-- "+enc)
	if err != nil {
		return "", err
	}
	if r.Error != nil {
		return "", fmt.Errorf("eval error %s: %s", r.Error.Code, r.Error.Message)
	}
	if len(r.Props) == 0 {
		return "(no result)", nil
	}
	return summarize(r.Props[0]), nil
}

func (s *session) PropertyGet(name string, depth int) (string, error) {
	r, _, err := s.cmd("property_get", fmt.Sprintf("-d %d -n %s", depth, name))
	if err != nil {
		return "", err
	}
	if len(r.Props) == 0 {
		return "(not found)", nil
	}
	return summarize(r.Props[0]), nil
}

func (s *session) PropertySet(name, value string) (string, error) {
	enc := base64.StdEncoding.EncodeToString([]byte(value))
	if _, _, err := s.cmd("property_set", fmt.Sprintf("-n %s -- %s", name, enc)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s = %s", name, value), nil
}

func (s *session) Detach() (string, error) {
	s.cmd("detach", "")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.state = "no session"
	return "detached", nil
}

func (s *session) Stop() (string, error) {
	s.cmd("stop", "")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.state = "no session"
	return "stopped", nil
}

func (s *session) Raw(cmd string) (string, error) {
	parts := strings.SplitN(cmd, " ", 2)
	args := ""
	if len(parts) == 2 {
		args = parts[1]
	}
	_, xmlStr, err := s.cmd(parts[0], args)
	return xmlStr, err
}

// ListenWait blocks until the next engine connection is adopted, or timeout.
func (s *session) ListenWait(timeout time.Duration) (string, error) {
	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()
	select {
	case <-ready:
		return s.Status(), nil
	case <-time.After(timeout):
		return "", fmt.Errorf("no engine connected within %s", timeout)
	}
}
