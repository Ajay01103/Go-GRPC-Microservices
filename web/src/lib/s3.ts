import "server-only"

import {
  DeleteObjectCommand,
  GetObjectCommand,
  PutObjectCommand,
  S3Client,
} from "@aws-sdk/client-s3"
import { getSignedUrl } from "@aws-sdk/s3-request-presigner"

const s3Endpoint = process.env.S3_ENDPOINT
const s3Region = process.env.S3_REGION
const s3Bucket = process.env.S3_BUCKET
const s3AccessKey = process.env.S3_ACCESS_KEY_ID
const s3SecretKey = process.env.S3_SECRET_ACCESS_KEY

if (!s3Endpoint || !s3Region || !s3Bucket || !s3AccessKey || !s3SecretKey) {
  throw new Error("Missing Backblaze B2 S3 environment variables")
}

const b2 = new S3Client({
  region: s3Region,
  endpoint: s3Endpoint,
  forcePathStyle: true,
  credentials: {
    accessKeyId: s3AccessKey,
    secretAccessKey: s3SecretKey,
  },
})

type UploadAudioOptions = {
  buffer: Buffer
  key: string
  contentType?: string
}

export async function uploadAudio({
  buffer,
  key,
  contentType = "audio/wav",
}: UploadAudioOptions): Promise<void> {
  await b2.send(
    new PutObjectCommand({
      Bucket: s3Bucket,
      Key: key,
      Body: buffer,
      ContentType: contentType,
    }),
  )
}

export async function deleteAudio(key: string): Promise<void> {
  await b2.send(
    new DeleteObjectCommand({
      Bucket: s3Bucket,
      Key: key,
    }),
  )
}

export async function getSignedAudioUrl(key: string): Promise<string> {
  const command = new GetObjectCommand({
    Bucket: s3Bucket,
    Key: key,
  })

  return getSignedUrl(b2, command, { expiresIn: 3600 })
}
