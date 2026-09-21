Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_signed_zero.c`

C++ (`conversions.cc`) :

```
inline double SignedZero(bool negative) {
  return negative ? base::uint64_to_double(base::Double::kSignMask) : 0.0;
}
```

C : `double c2_v8_parseint_signed_zero(int32_t negative);` — si `negative!=0` retourner `-0.0`, sinon `0.0`. SPDX Apache-2.0 OR MIT, stdint, CSG SignedZero. Un Write. Interdit Read/Grep/Glob.
