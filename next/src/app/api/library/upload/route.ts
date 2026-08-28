import { NextResponse } from "next/server";

// Placeholder: the upload flow isn't implemented yet. The previous version
// imported next-auth and @/lib/checkRoles, neither of which exists in this
// repo, which broke `next build`.
export async function POST(_req: Request) {
  return NextResponse.json({ error: "not implemented" }, { status: 501 });
}
