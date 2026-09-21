Write `/devhoros/c2simd/sources/c2archtsim/c2_v8_date_time_clip_trunc.c`

Recopie exactement dans `<c_file name="c2_v8_date_time_clip_trunc.c">` :

```
// SPDX-License-Identifier: Apache-2.0 OR MIT
#include <stdint.h>
// CSG DateCacheTryTimeClipTrunc
double c2_v8_date_time_clip_trunc(double time) {
    return (double)(int64_t)time + 0.0;
}
```

Rien d’autre.
