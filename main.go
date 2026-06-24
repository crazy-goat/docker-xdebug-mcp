// xdbg — a self-contained, docker-aware Xdebug (DBGp) debugger.
//
// Primary mode: an MCP stdio server exposing docker_xdebug_* tools (full HTTP
// method/header/body control for requests, host<->container path translation,
// CLI/command debugging). Spawned by an MCP client (e.g. Claude Code) via
// .mcp.json.
//
//	xdbg -dbg-port 9003 -local-root /Users/.../subscription-api -docker-root /var/www/subscription-api
//
// Secondary mode (no MCP client): a curl-driven HTTP control API.
//
//	xdbg -mcp=false -http 127.0.0.1:9010
package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	dbgPort := flag.String("dbg-port", "9003", "DBGp listen port (where container Xdebug connects)")
	localRoot := flag.String("local-root", "/Users/piotr.halas/work/subscription-api", "host project root")
	dockerRoot := flag.String("docker-root", "/var/www/subscription-api", "container project root")
	httpAddr := flag.String("http", "", "optional HTTP control API address, e.g. 127.0.0.1:9010")
	mcp := flag.Bool("mcp", true, "run as an MCP stdio server (stdout is the JSON-RPC channel)")
	flag.Parse()

	log.SetOutput(os.Stderr) // keep stdout clean for MCP JSON-RPC
	log.SetFlags(log.Ltime)

	s := newSession(*localRoot, *dockerRoot)
	if err := s.listen("0.0.0.0:" + *dbgPort); err != nil {
		log.Fatalf("listen: %v", err)
	}

	if *httpAddr != "" {
		go serveHTTP(s, *httpAddr)
	}

	if *mcp {
		log.Printf("MCP stdio server ready (docker_xdebug_*)")
		newMCP(s).serve() // blocks until stdin EOF
		return
	}
	select {} // HTTP-only mode
}
