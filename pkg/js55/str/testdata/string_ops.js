// Oracle Node des opérations de chaînes de js55.
//
// Protocole : lit sur l'entrée standard un JSON { cases: [[u16...], ...] } où
// chaque cas est une suite d'unités de code UTF-16. Écrit sur la sortie standard
// un JSON { results: [...] }, une entrée par cas.
//
// Ce fichier est la SOURCE : le test Go le lit depuis le disque et l'exécute,
// il ne le duplique pas dans une chaîne (interdit I2 du plan).

'use strict';

function fromUnits(units) {
  let s = '';
  for (let i = 0; i < units.length; i++) {
    s += String.fromCharCode(units[i]);
  }
  return s;
}

function toUnits(s) {
  const out = [];
  for (let i = 0; i < s.length; i++) out.push(s.charCodeAt(i));
  return out;
}

function isAscii(s) {
  for (let i = 0; i < s.length; i++) if (s.charCodeAt(i) >= 0x80) return false;
  return true;
}

function asciiUpper(s) {
  let out = '';
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    out += String.fromCharCode(c >= 97 && c <= 122 ? c - 32 : c);
  }
  return out;
}

let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (d) => { input += d; });
process.stdin.on('end', () => {
  const { cases } = JSON.parse(input);
  const results = [];

  for (let i = 0; i < cases.length; i++) {
    const s = fromUnits(cases[i]);
    const n = s.length;

    const charCodes = [];
    for (let k = 0; k < n; k++) charCodes.push(s.charCodeAt(k));

    const codePoints = [];
    for (let k = 0; k < n; k++) codePoints.push(s.codePointAt(k));

    // Concaténation avec le cas suivant, en boucle sur le corpus.
    const t = fromUnits(cases[(i + 1) % cases.length]);

    results.push({
      length: n,
      charCodes: charCodes,
      codePoints: codePoints,
      // slice sur des bornes déjà normalisées, comme l'API interne du moteur
      slice_1_n1: toUnits(n >= 2 ? s.slice(1, n - 1) : ''),
      indexOf_self: s.indexOf(s),
      indexOf_t: s.indexOf(t),
      concat: toUnits(s + t),
      cmp_t: s < t ? -1 : (s > t ? 1 : 0),
      eq_t: s === t,
      isAscii: isAscii(s),
      // Casse ASCII stricte : la fonction interne n'agit que sur les chaînes
      // purement ASCII, et rend la chaîne inchangée sinon.
      upperAscii: toUnits(isAscii(s) ? asciiUpper(s) : s),
    });
  }

  process.stdout.write(JSON.stringify({ results: results }));
});
