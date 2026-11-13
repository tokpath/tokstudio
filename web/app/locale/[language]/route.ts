import { normalizeLocale } from "@/lib/i18n";
import { localeDictionary } from "@/lib/i18n-catalogs";

export async function GET(_request: Request, { params }: { params: Promise<{ language: string }> }) {
  const { language } = await params;
  const locale = normalizeLocale(language);
  if (locale !== language) return new Response(null, { status: 404 });
  return Response.json(localeDictionary(locale), { headers: { "Cache-Control": "public, max-age=300" } });
}
