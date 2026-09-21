Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_r16_exact.c`

Chiffre (inline, radix 16) :

```
if (c >= '0' && c <= '9') d = c - '0';
else if (c >= 'a' && c <= 'f') d = c - 'a' + 10;
else if (c >= 'A' && c <= 'F') d = c - 'A' + 10;
else d = -1;
```

C obligatoire :

```
// SPDX-License-Identifier: Apache-2.0 OR MIT
#include <stdint.h>
int32_t c2_v8_parseint_r16_exact(const uint8_t *s, int32_t n, uint64_t *out_number);
```

Boucle i=0..n-1, accumuler `number = number * 16 + d` tant que `(number >> 53) == 0` et d>=0. `*out_number = number`. Retour = i consommé, 0 si aucun chiffre. CSG InternalStringToIntDoubleR16Exact. Réponse = uniquement le fichier C dans `<c_file name="c2_v8_parseint_r16_exact.c">`.
