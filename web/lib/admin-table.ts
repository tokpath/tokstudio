export type Column<T> = { key: keyof T | string; header: string };

export function tableRows<T extends Record<string, unknown>>(items: T[], columns: Column<T>[]): string[][] {
  return items.map((item) =>
    columns.map((column) => {
      const value = item[column.key as keyof T];
      if (value == null) {
        return "";
      }
      return String(value);
    }),
  );
}

export function paginate<T>(items: T[], page = 1, pageSize = 20): T[] {
  const start = Math.max(0, (page - 1) * pageSize);
  return items.slice(start, start + pageSize);
}
