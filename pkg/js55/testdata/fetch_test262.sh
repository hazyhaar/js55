#!/usr/bin/env bash
# Reconstitue l'arbre test262 au commit épinglé dans TEST262_PIN.
# L'arbre n'est pas suivi par git (cf. .gitignore) : seul le pin l'est.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PIN_FILE="$HERE/TEST262_PIN"
DST="$HERE/test262"

[ -f "$PIN_FILE" ] || { echo "TEST262_PIN absent : $PIN_FILE" >&2; exit 1; }
SHA="$(tr -d '[:space:]' < "$PIN_FILE")"
[ -n "$SHA" ] || { echo "TEST262_PIN vide" >&2; exit 1; }

if [ -d "$DST" ]; then
  echo "Arbre déjà présent : $DST"
  echo "Le supprimer explicitement pour reconstituer."
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Récupération de test262 au commit $SHA"
git -C "$TMP" init -q
git -C "$TMP" remote add origin https://github.com/tc39/test262.git
git -C "$TMP" fetch -q --depth 1 origin "$SHA"
git -C "$TMP" checkout -q FETCH_HEAD

GOT="$(git -C "$TMP" rev-parse HEAD)"
[ "$GOT" = "$SHA" ] || { echo "SHA obtenu $GOT != épinglé $SHA" >&2; exit 1; }

mkdir -p "$DST"
cp -a "$TMP/test" "$TMP/harness" "$DST/"
cp -a "$TMP/INTERPRETING.md" "$TMP/LICENSE" "$TMP/features.txt" "$TMP/excludelist.xml" "$DST/"

echo "Arbre reconstitué : $DST"
echo "Fichiers .js sous test/ : $(find "$DST/test" -name '*.js' | wc -l)"
