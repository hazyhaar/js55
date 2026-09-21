Id C10. Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_date_time_in_day.c`

C++ (`date.h` `DateCache::TimeInDay`) :

```
static const int64_t kMsPerDay = 24 * 60 * 60 * 1000;
static int TimeInDay(int64_t time_ms, int days) {
  return static_cast<int>(time_ms - days * kMsPerDay);
}
```

C : `int32_t c2_v8_date_time_in_day(int64_t time_ms, int32_t days);` SPDX, stdint, CSG DateCacheTimeInDay. Un Write. Interdit Read/Grep/Glob.
