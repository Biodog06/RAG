# Transport Layer Security (TLS)

## 1. Description
Transport Layer Security (TLS) is a cryptographic protocol designed to provide communications security over a computer network. It is the successor to Secure Sockets Layer (SSL).

## 2. Goals of TLS
- **Confidentiality**: Data is encrypted to hide it from eavesdroppers.
- **Integrity**: Data cannot be modified without detection.
- **Authentication**: Ensuring the parties are who they claim to be.

## 3. The TLS Handshake (Version 1.3)
TLS 1.3 is simpler and more secure than previous versions, reducing the handshake to a single round-trip (1-RTT).
1. **Client Hello**: Client sends supported ciphers, key share, and random number.
2. **Server Hello**: Server chooses cipher, sends its own key share, its certificate, and a "Finished" signature.
3. **Key Derivation**: Both parties derive the session key from the key shares.
4. **Encrypted Data**: Communication starts using the derived symmetric session keys.

## 4. Key Improvements in TLS 1.3
- **Removal of weak algorithms**: SHA-1, MD5, DES, and RC4 are gone.
- **PFS by Default**: Diffie-Hellman ephemeral is mandatory, ensuring Perfect Forward Secrecy.
- **0-RTT Mode**: Allows clients to send data in the first message if they have connected to the server before.
- **Encryption of handshake messages**: More metadata is hidden from observers.

## 5. Certificates and CAs
TLS relies on the Public Key Infrastructure (PKI). Entities called Certificate Authorities (CAs) sign digital certificates (X.509) to vouch for the ownership of a public key.

## 6. Common Attacks
- **Man-in-the-Middle (MITM)**: If a CA is compromised or a user ignores certificate warnings.
- **Protocol Downgrade**: Forcing a connection to use older, insecure SSL/TLS versions.
- **Heartbleed**: A memory leak bug in OpenSSL's heartbeat extension (fixed).
