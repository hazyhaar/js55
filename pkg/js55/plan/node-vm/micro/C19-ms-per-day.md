Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_date_ms_per_day.c`

C++ (`date.h`) :

```
static const int kSecPerDay = 24 * 60 * 60;
static const int64_t kMsPerDay = kSecPerDay * 1000;
```

C : `int64_t c2_v8_date_ms_per_day(void);` retourne `86400000`. SPDX, stdint, CSG DateCacheKMsPerDay. `<c_file name="c2_v8_date_ms_per_day.c">`.
