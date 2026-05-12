# Homomorphic Encryption

## 1. Overview
Homomorphic encryption (HE) is a form of encryption that allows computations to be performed on ciphertext, generating an encrypted result which, when decrypted, matches the result of the operations as if they had been performed on the plaintext.

## 2. Types of Homomorphic Encryption
- **Partially Homomorphic Encryption (PHE)**: Supports only one type of operation (either addition or multiplication). 
    - Example: RSA (multiplication), Paillier (addition).
- **Somewhat Homomorphic Encryption (SHE)**: Supports a limited number of both addition and multiplication operations. The number of operations is limited by "noise" that grows with each calculation until it prevents decryption.
- **Fully Homomorphic Encryption (FHE)**: Supports arbitrary computations (both addition and multiplication) of any depth. First achieved by Craig Gentry in 2009 using a technique called "bootstrapping" to reset the noise.

## 3. Use Cases
- **Cloud Computing Privacy**: Users can send encrypted data to a cloud server, have the server process it (e.g., search, analyze), and return the encrypted result without the server ever seeing the raw data.
- **Financial Services**: Private analysis of credit scores or insurance risks.
- **Healthcare**: Secure processing of genomic data and medical records for research without sharing patient identities.
- **Privacy-Preserving Machine Learning (PPML)**: Training models or performing inference on encrypted data.

## 4. Performance vs. Security
The main barrier to widespread adoption of FHE is performance. Computations on encrypted data are many orders of magnitude slower than on plaintext. However, specialized hardware (ASICs, FPGAs) and algorithmic improvements (like BGV, BFV, and CKKS schemes) are narrowing this gap.

## 5. Software Libraries
Popular libraries for HE include:
- **Microsoft SEAL**
- **PALISADE / OpenFHE**
- **HElib**
- **Concrete (Zama)**
