# Secure Multi-Party Computation (MPC)

## 1. Goal
Secure multi-party computation (also known as secure computation, multi-party computation, or MPC) is a subfield of cryptography with the goal of creating methods for parties to jointly compute a function over their inputs while keeping those inputs private.

## 2. Core Properties
- **Privacy**: No party should learn anything more than its prescribed output.
- **Correctness**: The output should be correct according to the function being computed.

## 3. Classic Example: Yao's Millionaires' Problem
Introduced in 1982 by Andrew Yao, the problem involves two millionaires who want to know which of them is richer without revealing their actual net worth to each other.

## 4. Key Techniques
- **Garbled Circuits**: A technique where a boolean circuit is "masked" or "garbled" so that it can be evaluated without revealing the gate inputs.
- **Oblivious Transfer (OT)**: A protocol where a sender transfers one of potentially many pieces of information to a receiver, but remains oblivious as to what piece was transferred.
- **Secret Sharing**: Dividing a secret into "shares" distributed among participants. Only a threshold of participants can reconstruct the secret (e.g., Shamir's Secret Sharing).

## 5. Modern Applications
- **Privacy-Preserving Auctions**: Bidders can find out who won without revealing their bids.
- **Distributed Key Management (Threshold Cryptography)**: Storing a private key by splitting it among multiple servers so that no single server has the full key and a threshold is required to sign or decrypt.
- **Privacy-Preserving Data Analysis**: Computing statistics on sensitive data sets from different organizations (e.g., finding the average salary across different companies).
