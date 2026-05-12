# Elliptic Curve Cryptography (ECC)

## 1. Introduction
Elliptic Curve Cryptography (ECC) is a public-key cryptography approach based on the algebraic structure of elliptic curves over finite fields. ECC allows smaller keys compared to non-EC cryptography (such as RSA) to provide equivalent security.

## 2. Mathematical Foundation
The security of ECC depends on the ability to compute a point multiplication and the inability to compute the inverse (the Elliptic Curve Discrete Logarithm Problem or ECDLP).
The equation for an elliptic curve is typically written as $y^2 = x^3 + ax + b$.

## 3. Comparison with RSA
ECC provides similar security levels with significantly smaller key sizes:
- **AES-128 security**: RSA-3072 vs. ECC-256 bits.
- **AES-256 security**: RSA-15360 vs. ECC-521 bits.
Smaller keys result in faster computations, lower power consumption, and less storage, making ECC ideal for mobile devices and IoT.

## 4. Common Curves
- **P-256 (nistp256)**: Standardized by NIST.
- **Curve25519**: Developed by Daniel J. Bernstein, known for high performance and security against side-channel attacks. Used in Signal, WhatsApp, and SSH.
- **secp256k1**: Used in Bitcoin and Ethereum.

## 5. Applications
- **ECDSA (Digital Signatures)**: Used in TLS certificates and blockchain transactions.
- **ECDH (Key Exchange)**: Used for establishing shared secrets over insecure channels.
- **EdDSA**: High-speed digital signatures using Edwards curves (e.g., Ed25519).

## 6. Implementation Risks
While mathematically strong, ECC implementations must be careful of invalid curve attacks, timing attacks, and the use of weak random number generators (as seen in the Sony PlayStation 3 ECDSA hack).
