// Cash Book customer search: filter + rank for the counter-side pick-list.
//
// Pure and synchronous so it can be unit-tested without the page. The ranked
// order is the point: a cashier types a few characters and expects the right
// person first, but the underlying list arrives name-alphabetical, which puts
// a customer with no debt above the one who owes.
export function searchClients<T extends Record<string, any>>(
  clients: T[],
  needle: string,
  outstandingByClient: Map<string, number>,
): T[] {
  const term = needle.trim().toLowerCase();
  if (!term) return [];

  // Exact name, then a name prefix, then an exact mobile/identity number, then
  // any other substring — so typing a full phone number ranks that customer
  // at the top even when their name doesn't contain the digits.
  const rank = (c: T): number => {
    const name = String(c.name ?? "").toLowerCase();
    if (name === term) return 0;
    if (name.startsWith(term)) return 1;
    if (String(c.phone ?? "").toLowerCase() === term) return 2;
    if (String(c.identity_number ?? "").toLowerCase() === term) return 3;
    return 4;
  };

  return clients
    .filter((c) =>
      // c.code (customer no.) and the secondary phones are searchable too —
      // the row displays them, so typing one must find it.
      [c.name, c.code, c.phone, c.phone2, c.phone3, c.identity_number, c.iban].some(
        (field) => field && String(field).toLowerCase().includes(term),
      ),
    )
    .sort((a, b) => {
      const byRank = rank(a) - rank(b);
      if (byRank !== 0) return byRank;
      // Within a tier, the customer who owes the most comes first; ties go
      // alphabetical so the order is deterministic.
      const debt = (outstandingByClient.get(b.id) ?? 0) - (outstandingByClient.get(a.id) ?? 0);
      if (debt !== 0) return debt;
      const byName = String(a.name ?? "").localeCompare(String(b.name ?? ""), undefined, {
        numeric: true,
      });
      // Then by id, so two identical names still order deterministically
      // rather than depending on the caller's array order.
      return byName !== 0 ? byName : String(a.id).localeCompare(String(b.id));
    });
}
