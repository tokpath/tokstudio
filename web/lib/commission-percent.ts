/** Money and ratios are converted at the API boundary; business forms use %. */
export function bpsToPercent(value: number): string {
  return String(value / 100);
}

export function percentToBps(raw: string): number | null {
  if (!/^\d+(\.\d{1,2})?$/.test(raw.trim())) return null;
  const value = Number(raw) * 100;
  return Number.isSafeInteger(Math.round(value)) && value >= 0 && value <= 10000 ? Math.round(value) : null;
}
