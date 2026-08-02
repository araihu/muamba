import { landingHtml } from "./_generated/html";

export async function GET() {
  return new Response(landingHtml, {
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "public, max-age=300, s-maxage=3600",
    },
  });
}
