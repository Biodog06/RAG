# Cryptocurrency Wallet and Key Security

## 1. Introduction
In the world of blockchain and cryptocurrency, "not your keys, not your coins" is a common mantra. Security relies entirely on the management of private keys.

## 2. Seed Phrases (BIP-39)
Most modern wallets generate a "mnemonic seed phrase" (12 or 24 words) which is used to derive all private keys for the wallet. This is based on the BIP-39 standard. Losing this phrase means losing access to all funds.

## 3. Types of Wallets
- **Hot Wallets**: Connected to the internet (e.g., browser extensions, mobile apps). Convenient but vulnerable to malware.
- **Cold Wallets**: Keep private keys offline.
    - **Hardware Wallets**: Specialized devices (e.g., Ledger, Trezor) that sign transactions internally without exposing keys to the computer.
    - **Paper Wallets**: Printing the keys on physical paper.

## 4. Multi-Signature (Multi-Sig)
Requires multiple private keys to authorize a transaction (e.g., a "2-of-3" wallet where 2 signatures are needed). This adds a layer of security, as compromising a single key is insufficient to steal funds.

## 5. Account Abstraction (ERC-4337)
An Ethereum standard that allows users to use smart contract wallets. This enables features like social recovery (recovering a wallet via trusted friends), custom gas payment logic, and batching transactions.

## 6. Common Threats
- **Phishing**: Trickery to get users to enter their seed phrase on a fake website.
- **Clipboard Hijacking**: Malware that replaces a copied crypto address with the attacker's address.
- **Dusting Attacks**: Sending tiny amounts of crypto to track the owner's wallet activity.
- **Smart Contract Vulnerabilities**: Bugs in the code of a Decentralized Finance (DeFi) protocol that allow attackers to drain funds.
