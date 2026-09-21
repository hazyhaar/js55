Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_parseint_round_even.c`

C++ :

```
int middle_value = (1 << (overflow_bits_count - 1));
if (dropped_bits > middle_value) {
  number++;
} else if (dropped_bits == middle_value) {
  if ((number & 1) != 0 || !zero_tail) {
    number++;
  }
}
```

C : `uint64_t c2_v8_parseint_round_even(uint64_t number, int32_t dropped_bits, int32_t overflow_bits_count, int32_t zero_tail);` — `zero_tail!=0` vrai. Ne pas toucher l’exposant. SPDX, stdint, CSG InternalStringToIntDoubleRoundEven. Un Write. Interdit Read/Grep/Glob.
