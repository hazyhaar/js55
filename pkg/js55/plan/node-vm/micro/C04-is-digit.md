Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_is_digit.c`

C++ (`conversions.cc`) :

```
inline bool isDigit(int x, int radix) {
  return (x >= '0' && x <= '9' && x < '0' + radix) ||
         (radix > 10 && x >= 'a' && x < 'a' + radix - 10) ||
         (radix > 10 && x >= 'A' && x < 'A' + radix - 10);
}
```

C : `int32_t c2_v8_parseint_is_digit(int32_t x, int32_t radix);` 1 ou 0. SPDX, stdint, CSG isDigit. Un Write. Interdit Read/Grep/Glob.
