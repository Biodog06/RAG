# Post-Quantum Cryptography (PQC)

## 1. Introduction
Post-quantum cryptography (sometimes referred to as quantum-proof, quantum-safe or quantum-resistant cryptography) refers to cryptographic algorithms (usually public-key algorithms) that are thought to be secure against a cryptanalytic attack by a quantum computer.

## 2. The Threat of Quantum Computers
The problem with currently popular algorithms is that their security relies on one of three hard mathematical problems: the integer factorization problem, the discrete logarithm problem or the elliptic-curve discrete logarithm problem. All of these problems can be easily solved on a sufficiently powerful quantum computer running Shor's algorithm.

## 3. Main Families of PQC
Currently, there are several main categories of quantum-resistant algorithms:
- **Lattice-based cryptography**: Schemes based on the hardness of lattice problems, such as the shortest vector problem (SVP). Examples include Kyber and Dilithium.
- **Code-based cryptography**: Based on error-correcting codes, such as the McEliece cryptosystem.
- **Multivariate cryptography**: Based on the difficulty of solving systems of multivariate quadratic equations.
- **Hash-based cryptography**: Based on the security of cryptographic hash functions (e.g., XMSS, SPHINCS+).
- **Isogeny-based cryptography**: Based on the properties of supersingular isogeny graphs.

## 4. NIST Standardization
The National Institute of Standards and Technology (NIST) initiated a process to solicit, evaluate, and standardize one or more quantum-resistant public-key cryptographic algorithms. In 2022, NIST announced the first four winners:
- For Public-Key Encryption/KEM: CRYSTALS-Kyber.
- For Digital Signatures: CRYSTALS-Dilithium, Falcon, and SPHINCS+.

## 5. Transition Challenges
Moving to PQC involves significant engineering challenges, including larger key sizes, larger signature sizes, and increased computational overhead compared to classical RSA or ECC.
