import { NextResponse } from "next/server";
import { getServerSession } from "next-auth/next";
import { authOptions } from "@/app/api/auth/[...nextauth]/route";
import { checkRoles } from "@/lib/checkRoles";


export async function POST(req: Request) {
    
}