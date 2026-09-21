Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_digit.c`

C++ source (`/data/v8_upstream/src/numbers/conversions.cc`, `InternalStringToIntDouble`, bloc chiffre) :

```
int digit;
if (*current >= '0' && *current < lim_0) {
  digit = static_cast<char>(*current) - '0';
} else if (*current >= 'a' && *current < lim_a) {
  digit = static_cast<char>(*current) - 'a' + 10;
} else if (*current >= 'A' && *current < lim_A) {
  digit = static_cast<char>(*current) - 'A' + 10;
} else {
  // junk
}
lim_0 = '0' + (radix < 10 ? radix : 10);
lim_a = 'a' + (radix - 10);
lim_A = 'A' + (radix - 10);
```

C attendu :

- `int32_t c2_v8_parseint_digit(int32_t c, int32_t radix);`
- `radix` dans 2..36. Si `c` n’est pas un chiffre de cette base, retourner `-1`.
- Pas de libc. Types `int32_t` seulement.

En-tête SPDX + `#include <stdint.h>` + commentaire CSG `InternalStringToIntDoubleDigit` + chemin conversions.cc.

Interdit : Read, Grep, Glob. Un Write. Verdict : une ligne.
