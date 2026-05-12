# The Birthday Paradox and Hash Collisions

## 1. The Paradox
The birthday paradox (or birthday problem) concerns the probability that, in a set of $n$ randomly chosen people, some pair of them will have the same birthday.
By the pigeonhole principle, the probability reaches 100% when the number of people reaches 367. However, 99.9% probability is reached with just 70 people, and 50% probability with only 23 people.

## 2. Mathematical Reason
The paradox occurs because it is the number of *pairs* of people that matters, not the number of people relative to the number of days in a year. For $n$ people, there are $n(n-1)/2$ possible pairs.

## 3. Application in Hash Functions
In cryptography, the birthday paradox defines the "birthday attack," which is used to find collisions in hash functions.
If a hash function produces an output of $H$ bits, there are $2^H$ possible hash values. An attacker can expect to find a collision (two different inputs that produce the same hash) after about $\sqrt{2^H} = 2^{H/2}$ attempts.

## 4. Security Implications
This means that the effective security of a hash function against collisions is only half its bit length:
- **MD5**: 128-bit hash -> 64-bit collision security (broken).
- **SHA-1**: 160-bit hash -> 80-bit collision security (weak/broken).
- **SHA-256**: 256-bit hash -> 128-bit collision security (currently secure).

## 5. Mitigation
To maintain a 128-bit security level against collision attacks, a hash function must produce at least a 256-bit output.
