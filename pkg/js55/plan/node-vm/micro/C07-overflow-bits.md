Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_overflow_bits.c`

C++ :

```
int overflow = static_cast<int>(number >> 53);
int overflow_bits_count = 1;
while (overflow > 1) {
  overflow_bits_count++;
  overflow >>= 1;
}
```

C : `int32_t c2_v8_parseint_overflow_bits(uint64_t number);` — `overflow = number >> 53` ; si 0 retourner 0 ; sinon compter comme ci-dessus. SPDX, stdint, CSG InternalStringToIntDoubleOverflowBits. Un Write. Interdit Read/Grep/Glob.
