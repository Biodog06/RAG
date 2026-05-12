# Password Hashing: Salt and Pepper

## 1. The Goal
Storing passwords as plaintext is a catastrophic security failure. Instead, systems store a cryptographic hash of the password. However, simple hashing is vulnerable to precomputed attacks like Rainbow Tables.

## 2. Salt
A "salt" is a random string of data that is appended to a password before it is hashed.
- **Purpose**: To ensure that two users with the same password have different stored hashes. This prevents the use of Rainbow Tables, as the table would need to be recomputed for every possible salt.
- **Storage**: The salt is stored in the database alongside the hashed password.

## 3. Pepper
A "pepper" is similar to a salt but with one key difference: it is NOT stored in the database.
- **Storage**: It is usually stored in the application's configuration or a Secure Key Vault.
- **Purpose**: If an attacker steals the database but does not have the pepper from the application server, they cannot perform an offline brute-force attack on the passwords.

## 4. Key Stretching
Modern password hashing uses algorithms that are intentionally slow to compute, making brute-force attacks expensive:
- **Argon2**: The winner of the Password Hashing Competition (PHC). Recommended for most uses.
- **bcrypt**: A classic, time-tested choice.
- **scrypt**: Designed to be memory-hard to prevent ASIC-based attacks.

## 5. Security Best Practices
- Never use fast hashes like MD5 or SHA-1 for passwords.
- Always use a unique salt for every user.
- Use a high iteration count (work factor).
