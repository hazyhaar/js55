# Atomes — parité node:vm

Grain : une fonction C++ (ou un Edit Go de quelques lignes) recopiée dans la fiche → un fichier. C01 tenu.

## C — V8 `conversions.cc` / `date.h`

| Id | Fonction C++ | Cible | Statut |
| :--- | :--- | :--- | :--- |
| C01 | bloc chiffre `InternalStringToIntDouble` | `c2_v8_parseint_digit.c` | .c + gen |
| C02 | `SignedZero` | `C02-signed-zero.md` | .c + gen |
| C03 | `JunkStringValue` | `C03-junk-nan.md` | .c + gen |
| C04 | `isDigit` | `C04-is-digit.md` | .c + gen |
| C05 | zéros de tête | `C05-skip-zeros.md` | .c + gen |
| C06 | accumulateur radix 16 exact | `C06-r16-exact.md` | .c + gen |
| C07 | `overflow_bits_count` | `C07-overflow-bits.md` | .c + gen |
| C08 | arrondi pair overflow | `C08-round-even.md` | .c + gen |
| C09 | `DaysFromTime` | `C05-days-from-time.md` | .c + gen |
| C10 | `TimeInDay` | `C06-time-in-day.md` | .c + gen |
| C11 | `Weekday` | `C07-weekday.md` | .c seulement (symbole déjà dans `date_day_gen.go`) |
| C12 | `TryTimeClip` trunc | `C12-time-clip-trunc.md` | .c Ornith sans math.h + gen sgoiter |
| C13 | `IsLeap` | `C13-is-leap.md` | .c + gen |
| C14 | `isBinaryDigit` | `C14-is-binary-digit.md` | .c + gen |
| C15 | signe `DetectRadixInternal` | `C15-parseint-sign.md` | .c + gen |
| C16 | préfixe `0x` | `C16-detect-0x.md` | .c + gen |
| C17 | radix 2..36 | `C17-radix-ok.md` | .c + gen |
| C18 | jours dans l’année | `C18-days-in-year.md` | .c + gen |
| C19 | `kMsPerDay` | `C19-ms-per-day.md` | .c + gen |

sgoiter après le `.c` uniquement ; `*_gen.go` non édité à la main.

## G — reliquats Go des M01–M14

| Id | Reliquat | Fichier |
| :--- | :--- | :--- |
| G01 | `arguments.callee` | `G01-arguments-callee.md` | tenu |
| G02 | SPDX C01 Apache-2.0 OR MIT | `G02-c01-spdx.md` | tenu |

M01–M05, M07, M08 apply, M13, M14 : déjà au sol, pas réatomisés. M06 descripteurs : pas un atome (tas). M09/M10 restes Test262 : un atome C ci-dessus, pas un RelPrefix.
