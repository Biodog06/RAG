# RSA (Rivest-Shamir-Adleman)

## 1. Description
RSA is a public-key cryptosystem that is widely used for secure data transmission. It is based on the practical difficulty of the factorization of the product of two large prime numbers.

## 2. Key Generation
1. Choose two distinct large prime numbers $p$ and $q$.
2. Compute $n = n = pq$. $n$ is used as the modulus for both the public and private keys.
3. Compute the totient $\phi(n) = (p-1)(q-1)$.
4. Choose an integer $e$ such that $1 < e < \phi(n)$ and $\text{gcd}(e, \phi(n)) = 1$; ($e$ is the public exponent).
5. Determine $d$ as $d \equiv e^{-1} \pmod{\phi(n)}$; ($d$ is the private exponent).

Public Key: $(e, n)$
Private Key: $(d, n)$

## 3. Operations
- **Encryption**: $c = m^e \pmod n$
- **Decryption**: $m = c^d \pmod n$

## 4. Security
RSA's security relies on the Integer Factorization Problem. While $n$ is public, finding $p$ and $q$ from $n$ is computationally expensive for large $n$ (currently 2048-bit or 4096-bit keys are standard).

## 5. Padding
Standard RSA is "textbook RSA" and is insecure because it is deterministic. In practice, RSA must be used with a padding scheme:
- **PKCS#1 v1.5**: Legacy padding (vulnerable to bleichenbacher attacks).
- **OAEP (Optimal Asymmetric Encryption Padding)**: Modern, provably secure padding scheme.

## 6. RSA Signatures
Digital signatures with RSA work by the signer using their private key to "decrypt" a hash of the message: $s = \text{hash}(m)^d \pmod n$. Anyone can verify it using the public key by checking if $s^e \equiv \text{hash}(m) \pmod n$.
