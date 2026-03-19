# AES (Advanced Encryption Standard)

## 1. Context
The Advanced Encryption Standard (AES), also known as Rijndael, is a specification for the encryption of electronic data established by the U.S. NIST in 2001.

## 2. Technical Details
AES is a symmetric-key algorithm, meaning the same key is used for both encrypting and decrypting the data.
- **Block Size**: Fixed at 128 bits.
- **Key Sizes**: Supports 128, 192, and 256 bits.
- **Rounds**: The number of transformation rounds depends on the key size: 10 rounds for 128-bit, 12 rounds for 192-bit, and 14 rounds for 256-bit.

## 3. Transformation Steps
Each round (except the last) consists of four layers:
1. **SubBytes**: Non-linear substitution step using a lookup table (S-box).
2. **ShiftRows**: Transposition step where the last three rows of the state are shifted cyclically.
3. **MixColumns**: Mixing operation which operates on the columns of the state, combining the four bytes in each column.
4. **AddRoundKey**: Each byte of the state is combined with a block of the round key using bitwise XOR.

## 4. Modes of Operation
AES is usually used with a mode of operation to encrypt data larger than 128 bits:
- **ECB (Electronic Codebook)**: Insecure for most uses as it encrypts identical blocks into identical ciphertexts.
- **CBC (Cipher Block Chaining)**: Uses an Initialization Vector (IV) and chains blocks together.
- **GCM (Galois/Counter Mode)**: Highly recommended; provides both confidentiality and authentication (AEAD).

## 5. Hardware Acceleration
Modern CPUs often include **AES-NI** (AES New Instructions), which implements AES rounds in hardware for extremely high performance.
