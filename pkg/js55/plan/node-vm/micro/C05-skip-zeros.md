Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_skip_zeros.c`

C++ (`InternalStringToIntDouble`) :

```
while (*current == '0') {
  ++current;
  if (current == end) return SignedZero(negative);
}
```

C : `int32_t c2_v8_parseint_skip_zeros(const uint8_t *s, int32_t n);` — avance tant que `s[i]=='0'`, retourne l’indice (n si tout zéros). `s` non NULL, `n>=0`. SPDX, stdint, CSG InternalStringToIntDoubleSkipZeros. Un Write. Interdit Read/Grep/Glob.
