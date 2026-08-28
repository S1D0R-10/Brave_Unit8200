import { NextResponse } from "next/server";
import { S3Client, ListObjectsV2Command } from "@aws-sdk/client-s3";

const s3 = new S3Client({
  region: process.env.S3_REGION || "auto",
  endpoint: process.env.S3_ENDPOINT,
  forcePathStyle: true,
  credentials: {
    accessKeyId: process.env.S3_ACCESS_KEY as string,
    secretAccessKey: process.env.S3_SECRET_KEY as string,
  },
});

export async function GET() {
  try {
    const command = new ListObjectsV2Command({
      Bucket: process.env.S3_BUCKET_NAME,
    });

    const response = await s3.send(command);
    const objects = response.Contents || [];

    // Uploads are stored as "<uuid v4>-<original name>"; show the original name.
    const UUID_PREFIX = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}-/i;

    const documents = objects.map(obj => ({
      key: obj.Key,
      name: obj.Key?.replace(UUID_PREFIX, "") || obj.Key || "Unknown",
      kind: obj.Key?.split('.').pop()?.toUpperCase() || "FILE",
      accent: "yellow", // default accent
      added: obj.LastModified ? new Date(obj.LastModified).toLocaleDateString("pl-PL") : "Unknown",
      uses: "0",
      status: "Zindeksowany", // or from bucket metadata if needed
      size: obj.Size
    }));

    return NextResponse.json({ documents });
  } catch (error) {
    console.error("Error listing documents:", error);
    return NextResponse.json({ error: "Failed to list documents" }, { status: 500 });
  }
}
