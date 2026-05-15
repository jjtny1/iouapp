// Currency-aware money formatting.
//
// Amounts are integer cents — hundredths of the currency's major unit — for
// every currency the app supports (e.g. 410000 renders as ¥4,100). Intl picks
// the right symbol and fraction-digit count per currency.

export function formatMoney(cents: number, currency: string): string {
  const amount = cents / 100;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(amount);
  } catch {
    // Intl throws RangeError for an unrecognized currency code (e.g. "USDC").
    return `${amount.toFixed(2)} ${currency}`;
  }
}
