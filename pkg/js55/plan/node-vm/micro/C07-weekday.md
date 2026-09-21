Id C11. Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_date_weekday.c`

C++ (`date.h` weekday) :

```
int Weekday(int days) {
  int result = (days + 4) % 7;
  return result >= 0 ? result : result + 7;
}
```

(Si le `+ 7` n’est pas dans l’extrait amont, garder le modulo et le repli négatif : weekday ECMA 0=dimanche.)

C : `int32_t c2_v8_date_weekday(int32_t days);` SPDX, stdint, CSG DateCacheWeekday. Un Write. Interdit Read/Grep/Glob.
