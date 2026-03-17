# Hardware Security Modules (HSM)

## 1. Definition
A Hardware Security Module (HSM) is a physical computing device that safeguards and manages digital keys, performs encryption and decryption functions for digital signatures, strong authentication and other cryptographic functions.

## 2. Key Features
- **Tamper Resistance**: HSMs are designed to detect physical intrusion and will "zeroize" (destroy) keys if tampered with.
- **Secure Key Storage**: Keys never leave the HSM in unencrypted form. All operations using the key happen *inside* the secure hardware.
- **Dedicated Performance**: Offloads cryptographic processing from the main application servers.

## 3. Form Factors
- **Plug-in Cards**: PCIe cards for servers.
- **Network Attachments**: Standalone appliances connected via LAN.
- **USB Tokens**: Small devices used for personal digital signatures (e.g., YubiKey in some configurations).
- **Cloud HSM**: Virtualized HSM services offered by providers like AWS, Azure, and Google Cloud.

## 4. Common Use Cases
- **Root CA protection**: Protecting the private key of a Certificate Authority.
- **Financial Transactions**: Processing PINs and EMV chip data for credit cards.
- **DNSSEC**: Signing DNS records.
- **Database Encryption**: Managing the Master Encryption Key (MEK) for Transparent Data Encryption (TDE).

## 5. Standards
HSMs are often validated against government standards:
- **FIPS 140-2**: Levels 1 through 4 (Level 3 is common for commercial HSMs).
- **Common Criteria (CC)**: International standards for IT security.
