import { NextResponse, type NextRequest } from "next/server";

export function proxy(request: NextRequest) {
  if (request.cookies.has("halo_session")) return NextResponse.next();
  const url = new URL("/sign-in", request.url);
  url.searchParams.set("next", request.nextUrl.pathname + request.nextUrl.search);
  return NextResponse.redirect(url);
}

export const config = {
  matcher: ["/admin/:path*", "/account/:path*"],
};
