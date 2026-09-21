Id C09. Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_date_days_from_time.c`

C++ (`/data/v8_upstream/src/date/date.h` `DateCache::DaysFromTime`) :

```
static const int64_t kMsPerDay = 24 * 60 * 60 * 1000;
static int DaysFromTime(int64_t time_ms) {
  if (time_ms < 0) time_ms -= (kMsPerDay - 1);
  return static_cast<int>(time_ms / kMsPerDay);
}
```

C : `int32_t c2_v8_date_days_from_time(int64_t time_ms);` — SPDX, stdint, CSG DateCacheDaysFromTime. Division entière vers zéro comme en C. Interdit Read/Grep/Glob. Un Write.
