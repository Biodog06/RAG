# Steganography and Cryptanalysis

## 1. Steganography
Steganography is the practice of concealing a message, file, or image within another message, file, or image. Unlike cryptography which hides the *content* of a message, steganography hides the *existence* of the message.
- **LSB (Least Significant Bit)**: Hiding data in the least significant bits of image pixels or audio samples.
- **Digital Watermarking**: Embedding information in a signal to verify its authenticity or identity.

## 2. Cryptanalysis
Cryptanalysis is the study of analyzing information systems in order to study the hidden aspects of the systems. It is generally used to refer to the process of breaking cryptographic security systems.

## 3. Types of Attacks
- **Ciphertext-only attack**: The attacker has access only to a set of ciphertexts.
- **Known-plaintext attack**: The attacker has samples of both plaintext and its corresponding ciphertext.
- **Chosen-plaintext attack**: The attacker can choose arbitrary plaintexts to be encrypted and obtain the resulting ciphertexts.
- **Brute-force attack**: Trying every possible key until the correct one is found.
- **Side-channel attack**: Based on information gained from the physical implementation of a cryptosystem, such as power consumption, electromagnetic leaks, or timing information.

## 4. Modern Cryptanalysis
Modern cryptanalysis uses sophisticated mathematical techniques and massive computational power. 
- **Differential Cryptanalysis**: Studying how differences in information input can affect the resultant difference at the output.
- **Linear Cryptanalysis**: Finding linear approximations to the action of a cipher.

## 5. Frequency Analysis
Basic substitution ciphers can be broken easily using frequency analysis—counting the occurrences of letters or symbols in the ciphertext and comparing them to the predictable frequency of letters in a given language.
