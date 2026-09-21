Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_detect_0x.c`

C++ (`DetectRadixInternal`, radix 0) :

```
if (*current == '0') {
  ++current;
  if (*current == 'x' || *current == 'X') { radix_ = 16; ++current; }
}
```

C : `int32_t c2_v8_parseint_detect_0x(const uint8_t *s, int32_t n, int32_t *out_off);`
- si `n>=2` et `s[0]=='0'` et (`s[1]=='x'` ou `'X'`) : `*out_off=2`, retourner 16
- sinon : `*out_off=0`, retourner 0
SPDX, stdint, CSG DetectRadixInternal0x. `<c_file name="c2_v8_parseint_detect_0x.c">`.
