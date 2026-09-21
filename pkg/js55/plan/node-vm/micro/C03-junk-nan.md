Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_junk_nan.c`

C++ (`conversions.cc`) :

```
inline double JunkStringValue() {
  return base::bit_cast<double, uint64_t>(kQuietNaNMask);
}
```

C : `uint64_t c2_v8_parseint_junk_nan_bits(void);` retourne `0x7FF8000000000000ULL` (qNaN IEEE). Pas de union flottante obligatoire. SPDX, stdint, CSG JunkStringValue. Un Write. Interdit Read/Grep/Glob.
