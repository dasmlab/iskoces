package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dasmlab/iskoces/pkg/proto/v1"
	"github.com/sirupsen/logrus"
)

var (
	serverAddr = flag.String("addr", "localhost:50051", "gRPC server address")
	sourceLang = flag.String("source", "en", "Source language code (e.g., en, fr)")
	targetLang = flag.String("target", "fr", "Target language code (e.g., en, fr)")
	textFile   = flag.String("file", "", "Path to text file to translate")
	text       = flag.String("text", "", "Text to translate (if file not provided)")
)

func main() {
	flag.Parse()

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Read text to translate
	var textToTranslate string
	if *textFile != "" {
		// Read from file
		data, err := os.ReadFile(*textFile)
		if err != nil {
			logger.WithError(err).Fatalf("Failed to read file: %s", *textFile)
		}
		textToTranslate = string(data)
	} else if *text != "" {
		textToTranslate = *text
	} else {
		logger.Fatal("Either -file or -text must be provided")
	}

	if textToTranslate == "" {
		logger.Fatal("Text to translate is empty")
	}

	logger.WithFields(logrus.Fields{
		"server":      *serverAddr,
		"source_lang": *sourceLang,
		"target_lang": *targetLang,
		"text_length": len(textToTranslate),
	}).Info("Connecting to Iskoces server...")

	// Connect to gRPC server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.WithError(err).Fatal("Failed to connect to server")
	}
	defer conn.Close()

	client := nanabushv1.NewTranslationServiceClient(conn)

	// Register client (required by the service)
	logger.Info("Registering client...")
	registerResp, err := client.RegisterClient(ctx, &nanabushv1.RegisterClientRequest{
		ClientName:    "test-client",
		ClientVersion: "1.0.0",
		Namespace:     "test",
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to register client")
	}
	logger.WithFields(logrus.Fields{
		"client_id": registerResp.ClientId,
	}).Info("Client registered successfully")

	// Perform translation
	logger.Info("Translating text...")
	startTime := time.Now()

	// Convert language codes to proto format (uppercase for source, BCP 47 for target)
	sourceLangProto := toProtoLangCode(*sourceLang, true)
	targetLangProto := toProtoLangCode(*targetLang, false)

	translateResp, err := client.Translate(ctx, &nanabushv1.TranslateRequest{
		JobId:          fmt.Sprintf("test-%d", time.Now().Unix()),
		Namespace:      "test",
		Primitive:      nanabushv1.PrimitiveType_PRIMITIVE_DOC_TRANSLATE,
		SourceLanguage: sourceLangProto,
		TargetLanguage: targetLangProto,
		Source: &nanabushv1.TranslateRequest_Doc{
			Doc: &nanabushv1.DocumentContent{
				Title:    "Test Translation",
				Markdown: textToTranslate,
			},
		},
	})
	if err != nil {
		logger.WithError(err).Fatal("Translation failed")
	}

	duration := time.Since(startTime)

	if !translateResp.Success {
		logger.WithFields(logrus.Fields{
			"error": translateResp.ErrorMessage,
		}).Fatal("Translation was not successful")
	}

	// Output results
	separator := strings.Repeat("=", 80)
	dashLine := strings.Repeat("-", 80)
	
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("TRANSLATION RESULTS")
	fmt.Println(separator)
	fmt.Printf("\nSource Language: %s (%s)\n", *sourceLang, sourceLangProto)
	fmt.Printf("Target Language: %s (%s)\n", *targetLang, targetLangProto)
	fmt.Printf("Translation Time: %.2f seconds\n", translateResp.InferenceTimeSeconds)
	fmt.Println()
	fmt.Println(dashLine)
	fmt.Println("ORIGINAL TEXT:")
	fmt.Println(dashLine)
	fmt.Println(textToTranslate)
	fmt.Println()
	fmt.Println(dashLine)
	fmt.Println("TRANSLATED TEXT:")
	fmt.Println(dashLine)
	fmt.Println(translateResp.TranslatedMarkdown)
	fmt.Println()
	fmt.Println(separator)

	logger.WithFields(logrus.Fields{
		"duration_seconds": duration.Seconds(),
		"success":          translateResp.Success,
	}).Info("Translation completed successfully")
}

// toProtoLangCode converts a language code to proto format.
// For source languages, use uppercase (e.g., "en" -> "EN")
// For target languages, use BCP 47 format (e.g., "fr" -> "fr-CA" or just "fr")
func toProtoLangCode(lang string, isSource bool) string {
	if isSource {
		// Source languages are uppercase in proto
		return toUpper(lang)
	}
	// Target languages can be BCP 47, but we'll use lowercase base code
	// The service will handle conversion
	return lang
}

// toUpper converts a string to uppercase (simple implementation)
func toUpper(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'a' && r <= 'z' {
			result[i] = r - 32
		} else {
			result[i] = r
		}
	}
	return string(result)
}

