# Diffie-Hellman Key Exchange

## 1. Context
The Diffie-Hellman (DH) key exchange is a method of securely exchanging cryptographic keys over a public channel. It was one of the first public-key protocols as conceived by Whitfield Diffie and Martin Hellman in 1976.

## 2. The Protocol Logic
DH allows two parties, Alice and Bob, to establish a shared secret without ever having met.
1. Alice and Bob agree on a public prime number $p$ and a generator $g$.
2. Alice chooses a private secret $a$ and calculates $A = g^a \mod p$. She sends $A$ to Bob.
3. Bob chooses a private secret $b$ and calculates $B = g^b \mod p$. He sends $B$ to Alice.
4. Alice calculates $S = B^a \mod p$.
5. Bob calculates $S = A^b \mod p$.
Since $S = (g^b)^a = (g^a)^b = g^{ab} \mod p$, they now have a shared secret $S$.

## 3. Security Basis
DH is secure because calculating the discrete logarithm of $g^{ab} \mod p$ is computationally infeasible for large primes, whereas exponentiation is relatively fast.

## 4. Perfect Forward Secrecy (PFS)
When DH keys are generated for each session (Diffie-Hellman Ephemeral or DHE), even if the server's long-term private key is stolen later, past session communications cannot be decrypted. This property is known as Perfect Forward Secrecy.

## 5. Vulnerabilities
- **Logjam Attack**: Exploits the use of weak 512-bit prime numbers (export-grade cryptography).
- **Man-in-the-Middle**: Basic DH does not provide authentication. An attacker could perform separate DH exchanges with Alice and Bob to sit in the middle of their conversation. Digital signatures are usually used to mitigate this.
