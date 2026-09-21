Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_sign.c`

C++ (`DetectRadixInternal`) :

```
if (*current == '+') { ++current; sign_ = Sign::kPositive; }
else if (*current == '-') { ++current; sign_ = Sign::kNegative; }
```

C : `int32_t c2_v8_parseint_sign(const uint8_t *s, int32_t n, int32_t *out_off);`
- `n<=0` : `*out_off=0`, retourner 1
- `s[0]=='+'` : `*out_off=1`, retourner 1
- `s[0]=='-'` : `*out_off=1`, retourner -1
- sinon : `*out_off=0`, retourner 1
SPDX, stdint, CSG DetectRadixInternalSign. `<c_file name="c2_v8_parseint_sign.c">`.
