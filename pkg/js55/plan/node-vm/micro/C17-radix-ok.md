Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_radix_ok.c`

C++ (`DetectRadixInternal`) :

```
DCHECK(radix_ >= 2 && radix_ <= 36);
```

C : `int32_t c2_v8_parseint_radix_ok(int32_t radix);` 1 si `2 <= radix && radix <= 36`, sinon 0. SPDX, stdint, CSG ParseIntRadixRange. `<c_file name="c2_v8_parseint_radix_ok.c">`.
