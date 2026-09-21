Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_date_is_leap.c`

C++ (`date.h`) :

```
bool IsLeap(int year) {
  return year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
}
```

C : `int32_t c2_v8_date_is_leap(int32_t year);` 1 ou 0. SPDX, stdint, CSG DateCacheIsLeap. Un Write. Interdit Read/Grep/Glob.
