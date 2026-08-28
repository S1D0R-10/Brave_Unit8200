import { NextRequest, NextResponse } from "next/server";
import { S3Client, GetObjectCommand } from "@aws-sdk/client-s3";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";

// Opens the original uploaded object: redirects the browser to a short-lived
// presigned GET, so the file itself never streams through this app.
const s3 = new S3Client({
  region: process.env.S3_REGION || "auto",
  endpoint: process.env.S3_ENDPOINT,
  forcePathStyle: true,
  credentials: {
    accessKeyId: process.env.S3_ACCESS_KEY as string,
    secretAccessKey: process.env.S3_SECRET_KEY as string,
  },
});

export async function GET(request: NextRequest) {
  const key = request.nextUrl.searchParams.get("key");
  if (!key) {
    return NextResponse.json({ error: "key is required" }, { status: 400 });
  }

  try {
    const command = new GetObjectCommand({
      Bucket: process.env.S3_BUCKET_NAME,
      Key: key,
      ResponseContentDisposition: "inline",
    });
    const presignedUrl = await getSignedUrl(s3, command, { expiresIn: 600 });
    return NextResponse.redirect(presignedUrl);
  } catch (error) {
    console.error("Error presigning file URL:", error);
    return NextResponse.json({ error: "Failed to open the file" }, { status: 500 });
  }
}
