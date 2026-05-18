import { NextResponse, type NextRequest } from "next/server";

export function middleware(req: NextRequest) {
  const token = req.cookies.get("session_token")?.value;
  const isLogin = req.nextUrl.pathname === "/login";
  const isChange = req.nextUrl.pathname === "/change-password";
  const isAPIProxy = req.nextUrl.pathname.startsWith("/admin/");

  if (isAPIProxy) {
    return NextResponse.next();
  }

  if (!token && !isLogin) {
    return NextResponse.redirect(new URL("/login", req.url));
  }
  if (isChange || isLogin) {
    return NextResponse.next();
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
