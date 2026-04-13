package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	voiceconfig "github.com/go-grpc-sqlc/voice/config"
	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
)

const systemUserID = "SYSTEM"

type voiceMetadata struct {
	Description string
	Category    db.VoiceCategory
	Language    string
	Variant     db.VoiceVariant
}

var systemVoiceMetadata = map[string]voiceMetadata{
	"Aaron":    {Description: "Soothing and calm, like a self-help audiobook narrator", Category: db.VoiceCategoryNARRATION, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Abigail":  {Description: "Friendly and conversational with a warm, approachable tone", Category: db.VoiceCategoryGENERAL, Language: "en-GB", Variant: db.VoiceVariantFEMALE},
	"Anaya":    {Description: "Polite and professional, suited for customer service", Category: db.VoiceCategoryGENERAL, Language: "en-IN", Variant: db.VoiceVariantFEMALE},
	"Andy":     {Description: "Versatile and clear, a reliable all-purpose narrator", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Archer":   {Description: "Laid-back and reflective with a steady, storytelling pace", Category: db.VoiceCategoryNARRATION, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Brian":    {Description: "Professional and helpful with a clear customer support tone", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Chloe":    {Description: "Bright and bubbly with a cheerful, outgoing personality", Category: db.VoiceCategoryGENERAL, Language: "en-AU", Variant: db.VoiceVariantFEMALE},
	"Dylan":    {Description: "Thoughtful and intimate, like a quiet late-night conversation", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Emmanuel": {Description: "Nasally and distinctive with a quirky, cartoon-like quality", Category: db.VoiceCategoryCHARACTER, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Ethan":    {Description: "Polished and warm with crisp, studio-quality delivery", Category: db.VoiceCategoryNARRATION, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Evelyn":   {Description: "Warm Southern charm with a heartfelt, down-to-earth feel", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantFEMALE},
	"Gavin":    {Description: "Calm and reassuring with a smooth, natural flow", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Gordon":   {Description: "Warm and encouraging with an uplifting, motivational tone", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantMALE},
	"Ivan":     {Description: "Deep and cinematic with a dramatic, movie-character presence", Category: db.VoiceCategoryCHARACTER, Language: "ru-RU", Variant: db.VoiceVariantMALE},
	"Laura":    {Description: "Authentic and warm with a conversational Midwestern tone", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantFEMALE},
	"Lucy":     {Description: "Direct and composed with a professional phone manner", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantFEMALE},
	"Madison":  {Description: "Energetic and unfiltered with a casual, chatty vibe", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantFEMALE},
	"Marisol":  {Description: "Confident and polished with a persuasive, ad-ready delivery", Category: db.VoiceCategoryGENERAL, Language: "en-US", Variant: db.VoiceVariantFEMALE},
	"Meera":    {Description: "Friendly and helpful with a clear, service-oriented tone", Category: db.VoiceCategoryGENERAL, Language: "en-IN", Variant: db.VoiceVariantFEMALE},
	"Walter":   {Description: "Old and raspy with deep gravitas, like a wise grandfather", Category: db.VoiceCategoryNARRATION, Language: "en-US", Variant: db.VoiceVariantMALE},
}

var canonicalSystemVoiceNames = []string{
	"Aaron", "Abigail", "Anaya", "Andy", "Archer",
	"Brian", "Chloe", "Dylan", "Emmanuel", "Ethan",
	"Evelyn", "Gavin", "Gordon", "Ivan", "Laura",
	"Lucy", "Madison", "Marisol", "Meera", "Walter",
}

func mustValue(name, value string) string {
	if value == "" {
		log.Fatalf("missing required config value: %s", name)
	}
	return value
}

func systemVoicesDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "system-voices")
}

func readVoiceAudio(name string) ([]byte, error) {
	p := filepath.Join(systemVoicesDir(), name+".wav")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s.wav: %w", name, err)
	}
	return data, nil
}

func uploadToB2(ctx context.Context, client *s3.Client, bucket, key string, data []byte) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("audio/wav"),
	})
	return err
}

func seedSystemVoice(ctx context.Context, q *db.Queries, b2Client *s3.Client, bucket, name string) error {
	meta, hasMeta := systemVoiceMetadata[name]
	if !hasMeta {
		return fmt.Errorf("no metadata defined for voice %q", name)
	}

	audioData, err := readVoiceAudio(name)
	if err != nil {
		return err
	}

	existing, err := q.GetSystemVoiceByName(ctx, name)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("checking existing voice %q: %w", name, err)
	}

	// --- UPDATE PATH ---
	if err == nil {
		s3Key := fmt.Sprintf("system/%s.wav", existing.ID)

		if err := uploadToB2(ctx, b2Client, bucket, s3Key, audioData); err != nil {
			return fmt.Errorf("uploading audio for existing voice %q: %w", name, err)
		}

		return q.UpdateSystemVoiceS3Key(ctx, db.UpdateSystemVoiceS3KeyParams{
			ID:          existing.ID,
			S3ObjectKey: pgtype.Text{String: s3Key, Valid: true},
			Description: pgtype.Text{String: meta.Description, Valid: true},
			Category:    meta.Category,
			Language:    meta.Language,
		})
	}

	// --- CREATE PATH ---
	voice, err := q.CreateVoice(ctx, db.CreateVoiceParams{
		ID:      uuid.New().String(),
		Column2: systemUserID,
		Name:    name,
		Description: pgtype.Text{String: meta.Description, Valid: true},
		Category:    meta.Category,
		Language:    meta.Language,
		Variant:     meta.Variant,
		S3ObjectKey: pgtype.Text{Valid: false},
	})
	if err != nil {
		return fmt.Errorf("creating voice record for %q: %w", name, err)
	}

	s3Key := fmt.Sprintf("system/%s.wav", voice.ID)

	if err := uploadToB2(ctx, b2Client, bucket, s3Key, audioData); err != nil {
		_ = q.DeleteSystemVoiceByID(ctx, voice.ID)
		return fmt.Errorf("uploading audio for new voice %q: %w", name, err)
	}

	return q.UpdateSystemVoiceS3Key(ctx, db.UpdateSystemVoiceS3KeyParams{
		ID:          voice.ID,
		S3ObjectKey: pgtype.Text{String: s3Key, Valid: true},
		Description: pgtype.Text{String: meta.Description, Valid: true},
		Category:    meta.Category,
		Language:    meta.Language,
	})
}

func main() {
	cfg, err := voiceconfig.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dbURL := mustValue("VOICE_DB_URL", cfg.DBUrl)
	s3Endpoint := mustValue("S3_ENDPOINT", cfg.S3Endpoint)
	s3Region := mustValue("S3_REGION", cfg.S3Region)
	s3Bucket := mustValue("S3_BUCKET", cfg.S3Bucket)
	s3AccessKey := mustValue("S3_ACCESS_KEY_ID", cfg.S3AccessKey)
	s3SecretKey := mustValue("S3_SECRET_ACCESS_KEY", cfg.S3SecretKey)

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	queries := db.New(pool)

	b2Cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(s3Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(s3AccessKey, s3SecretKey, ""),
		),
	)
	if err != nil {
		log.Fatalf("B2 config: %v", err)
	}

	b2Client := s3.NewFromConfig(b2Cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s3Endpoint)
		o.UsePathStyle = true
	})

	fmt.Printf("Seeding %d system voices...\n", len(canonicalSystemVoiceNames))

	for _, name := range canonicalSystemVoiceNames {
		fmt.Printf("- %s\n", name)
		if err := seedSystemVoice(ctx, queries, b2Client, s3Bucket, name); err != nil {
			log.Fatalf("seed failed for %q: %v", name, err)
		}
	}

	fmt.Println("System voice seed completed.")
}
