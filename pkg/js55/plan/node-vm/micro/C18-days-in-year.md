Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_date_days_in_year.c`

C++ (`date.h` `IsLeap` + 365/366) :

```
bool IsLeap(int year) {
  return year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
}
// days = IsLeap(year) ? 366 : 365
```

C : `int32_t c2_v8_date_days_in_year(int32_t year);` 366 ou 365, leap **inline** (pas d’appel). SPDX, stdint, CSG DateCacheDaysInYear. `<c_file name="c2_v8_date_days_in_year.c">`.
