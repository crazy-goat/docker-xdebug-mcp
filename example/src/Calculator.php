<?php

namespace App;

class Calculator
{
    public function add(int $a, int $b): int
    {
        $result = $a + $b;

        if ($result > 100) {
            $this->logLargeResult($result);
        }

        return $result;
    }

    public function divide(int $a, int $b): float
    {
        if ($b === 0) {
            throw new \InvalidArgumentException('Division by zero');
        }

        return $a / $b;
    }

    private function logLargeResult(int $value): void
    {
        file_put_contents('php://stderr', "Large result: $value\n");
    }
}
