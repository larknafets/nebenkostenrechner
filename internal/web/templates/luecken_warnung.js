// Hint under the reading date when the gap to the last reading is longer than
// one month. A pure function of two "YYYY-MM-DD" strings: both dates are
// compared as UTC calendar days, so the result does not depend on the time zone
// of the browser (Date("2026-10-01") is UTC midnight, Date(2026, 9, 1) is
// local midnight, and mixing the two made exactly one month warn in CEST).
// Returns "" when there is nothing to warn about. The same file is run by
// luecken_warnung_test.go under Node.
function lueckenWarnung(vorher, neu) {
  const teile = (s) => {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s || '');
    return m && { j: +m[1], m: +m[2], t: +m[3] };
  };
  const v = teile(vorher), n = teile(neu);
  if (!v || !n) return '';

  const vorherTag = Date.UTC(v.j, v.m - 1, v.t);
  const neuTag = Date.UTC(n.j, n.m - 1, n.t);
  // Same day of the next month, clamped to its last day (31.01. + 1 month is
  // 28.02. or 29.02., not 03.03.).
  const letzterTagImZielmonat = new Date(Date.UTC(v.j, v.m + 1, 0)).getUTCDate();
  const einMonatSpaeter = Date.UTC(v.j, v.m, Math.min(v.t, letzterTagImZielmonat));
  if (neuTag <= einMonatSpaeter) return '';

  const tage = (neuTag - vorherTag) / 86400000;
  const monate = Math.round((tage / 30.44) * 10) / 10;
  const monateText = monate === 1 ? '1 Monat' : String(monate).replace('.', ',') + ' Monate';
  const pad = (x) => String(x).padStart(2, '0');
  return '⚠ Verbrauch über ' + monateText + ' (letzte Ablesung: ' + pad(v.t) + '.' + pad(v.m) + '.' + v.j + ')';
}
