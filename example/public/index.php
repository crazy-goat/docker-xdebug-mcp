<?php

require __DIR__ . '/../vendor/autoload.php';

use Slim\Factory\AppFactory;
use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;

$app = AppFactory::create();

$app->get('/', function (Request $request, Response $response) {
    $response->getBody()->write(json_encode([
        'status' => 'ok',
        'message' => 'xdbg demo is running'
    ]));
    return $response->withHeader('Content-Type', 'application/json');
});

$app->get('/hello/{name}', function (Request $request, Response $response, array $args) {
    $name = $args['name'];
    $response->getBody()->write(json_encode([
        'greeting' => "Hello, $name!"
    ]));
    return $response->withHeader('Content-Type', 'application/json');
});

$app->post('/echo', function (Request $request, Response $response) {
    $body = $request->getBody()->getContents();
    $data = json_decode($body, true) ?: [];
    $response->getBody()->write(json_encode([
        'received' => $data
    ]));
    return $response->withHeader('Content-Type', 'application/json');
});

$app->get('/calc/add/{a}/{b}', function (Request $request, Response $response, array $args) {
    $calc = new App\Calculator();
    $result = $calc->add((int) $args['a'], (int) $args['b']);
    $response->getBody()->write(json_encode(['result' => $result]));
    return $response->withHeader('Content-Type', 'application/json');
});

$app->get('/calc/divide/{a}/{b}', function (Request $request, Response $response, array $args) {
    $calc = new App\Calculator();
    $result = $calc->divide((int) $args['a'], (int) $args['b']);
    $response->getBody()->write(json_encode(['result' => $result]));
    return $response->withHeader('Content-Type', 'application/json');
});

$app->get('/users', [new App\UserController(), 'list']);
$app->get('/users/{id}', [new App\UserController(), 'get']);

$app->run();
