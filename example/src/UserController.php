<?php

namespace App;

use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;

class UserController
{
    private array $users = [
        1 => ['id' => 1, 'name' => 'Alice', 'email' => 'alice@example.com'],
        2 => ['id' => 2, 'name' => 'Bob', 'email' => 'bob@example.com'],
    ];

    public function list(Request $request, Response $response): Response
    {
        $response->getBody()->write(json_encode(array_values($this->users)));
        return $response->withHeader('Content-Type', 'application/json');
    }

    public function get(Request $request, Response $response, array $args): Response
    {
        $id = (int) $args['id'];

        if (!isset($this->users[$id])) {
            $response->getBody()->write(json_encode(['error' => 'User not found']));
            return $response->withStatus(404)->withHeader('Content-Type', 'application/json');
        }

        $response->getBody()->write(json_encode($this->users[$id]));
        return $response->withHeader('Content-Type', 'application/json');
    }
}
