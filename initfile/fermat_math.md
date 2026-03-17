# Fermat's Little Theorem and RSA

## 1. Theorem Definition
Fermat's Little Theorem states that if $p$ is a prime number, then for any integer $a$, the number $a^p - a$ is an integer multiple of $p$.
In notation: $a^p \equiv a \pmod p$.
If $a$ is not divisible by $p$, Fermat's Little Theorem is equivalent to the statement $a^{p-1} \equiv 1 \pmod p$.

## 2. Importance in Cryptography
This theorem is a fundamental pillar of modern public-key cryptography, particularly the RSA algorithm.

## 3. Primality Testing
The theorem forms the basis for the Fermat primality test, which is a simple probabilistic test to determine whether a number is prime. While it can produce "false positives" (Fermat pseudoprimes), it is a fast first step in most primality testing algorithms.

## 4. Euler's Generalization
Leonhard Euler generalized Fermat's Little Theorem in what is now called Euler's Totient Theorem:
$a^{\phi(n)} \equiv 1 \pmod n$ for any $a$ coprime to $n$.
This generalization is what RSA uses to ensure that $m^{ed} \equiv m \pmod n$, where $n$ is a product of two primes.

## 5. Practical Use
In RSA key generation, $e$ and $d$ are chosen such that $ed \equiv 1 \pmod{\phi(n)}$. The theorem guarantees that $m^{ed} \equiv m^1 \equiv m \pmod n$, ensuring the decryption process retrieves the original message.
