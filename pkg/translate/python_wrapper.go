package translate

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/sirupsen/logrus"
)

// PythonTranslator uses a Python subprocess to call LibreTranslate/Argos directly.
// This eliminates HTTP overhead and allows true streaming.
type PythonTranslator struct {
	engine      EngineType
	pythonPath  string
	scriptPath  string
	process     *exec.Cmd
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	mu          sync.Mutex
	logger      *logrus.Logger
	initialized bool
}

// NewPythonTranslator creates a new Python-based translator.
// It starts a Python subprocess that imports the translation library directly.
func NewPythonTranslator(engine EngineType, logger *logrus.Logger) (*PythonTranslator, error) {
	if logger == nil {
		logger = logrus.New()
	}

	// Determine script path based on engine
	scriptPath := "/app/scripts/translate_worker.py"
	
	pt := &PythonTranslator{
		engine:     engine,
		pythonPath: "python3",
		scriptPath: scriptPath,
		logger:     logger,
	}

	return pt, nil
}

// generatePythonScript generates the Python code that will be executed.
func (pt *PythonTranslator) generatePythonScript() string {
	var importStmt, translateCode string
	
	switch pt.engine {
	case EngineLibreTranslate:
		// LibreTranslate uses argostranslate under the hood
		importStmt = `
import sys
import json
import argostranslate.package
import argostranslate.translate
`
		translateCode = `
def translate_text(text, source_lang, target_lang):
    # Install/update packages if needed
    argostranslate.package.update_package_index()
    available_packages = argostranslate.package.get_available_packages()
    package_to_install = next(
        (pkg for pkg in available_packages 
         if pkg.from_code == source_lang and pkg.to_code == target_lang),
        None
    )
    if package_to_install:
        argostranslate.package.install_from_path(package_to_install.download())
    
    # Translate
    return argostranslate.translate.translate(text, source_lang, target_lang)
`
	case EngineArgos:
		importStmt = `
import sys
import json
import argostranslate.package
import argostranslate.translate
`
		translateCode = `
def translate_text(text, source_lang, target_lang):
    # Install/update packages if needed
    argostranslate.package.update_package_index()
    available_packages = argostranslate.package.get_available_packages()
    package_to_install = next(
        (pkg for pkg in available_packages 
         if pkg.from_code == source_lang and pkg.to_code == target_lang),
        None
    )
    if package_to_install:
        argostranslate.package.install_from_path(package_to_install.download())
    
    # Translate
    return argostranslate.translate.translate(text, source_lang, target_lang)
`
	default:
		return ""
	}

	return fmt.Sprintf(`%s
%s

# Main loop: read JSON from stdin, translate, write JSON to stdout
for line in sys.stdin:
    try:
        request = json.loads(line.strip())
        text = request.get('text', '')
        source_lang = request.get('source_lang', 'en')
        target_lang = request.get('target_lang', 'fr')
        
        translated = translate_text(text, source_lang, target_lang)
        
        response = {
            'success': True,
            'translated_text': translated
        }
        print(json.dumps(response))
        sys.stdout.flush()
    except Exception as e:
        error_response = {
            'success': False,
            'error': str(e)
        }
        print(json.dumps(error_response))
        sys.stdout.flush()
`, importStmt, translateCode)
}

// ensureProcess ensures the Python subprocess is running.
func (pt *PythonTranslator) ensureProcess(ctx context.Context) error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.initialized && pt.process != nil {
		// Check if process is still running
		if pt.process.ProcessState == nil || !pt.process.ProcessState.Exited() {
			return nil
		}
	}

	// Start Python subprocess with the worker script
	pt.process = exec.CommandContext(ctx, pt.pythonPath, pt.scriptPath)
	
	var err error
	pt.stdin, err = pt.process.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	
	pt.stdout, err = pt.process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	
	// Set stderr to capture errors
	pt.process.Stderr = pt.process.Stdout // For now, merge stderr with stdout
	
	if err := pt.process.Start(); err != nil {
		return fmt.Errorf("failed to start Python process: %w", err)
	}
	
	pt.initialized = true
	pt.logger.Info("Python translator subprocess started")
	
	return nil
}

// Translate translates text using the Python subprocess.
func (pt *PythonTranslator) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	if err := pt.ensureProcess(ctx); err != nil {
		return "", err
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Send request as JSON
	request := map[string]interface{}{
		"text":        text,
		"source_lang": sourceLang,
		"target_lang": targetLang,
	}
	
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// Write request to stdin
	if _, err := pt.stdin.Write(append(requestJSON, '\n')); err != nil {
		return "", fmt.Errorf("failed to write to stdin: %w", err)
	}
	
	// Read response from stdout
	scanner := bufio.NewScanner(pt.stdout)
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read response: %v", scanner.Err())
	}
	
	var response map[string]interface{}
	if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	if success, ok := response["success"].(bool); !ok || !success {
		errorMsg := "unknown error"
		if errStr, ok := response["error"].(string); ok {
			errorMsg = errStr
		}
		return "", fmt.Errorf("translation failed: %s", errorMsg)
	}
	
	translatedText, ok := response["translated_text"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format: translated_text not found")
	}
	
	return translatedText, nil
}

// CheckHealth verifies the Python translator is ready.
func (pt *PythonTranslator) CheckHealth(ctx context.Context) error {
	// Try to translate a simple test string
	_, err := pt.Translate(ctx, "test", "en", "fr")
	return err
}

// SupportedLanguages returns supported language codes.
func (pt *PythonTranslator) SupportedLanguages(ctx context.Context) ([]string, error) {
	// Common languages supported by Argos/LibreTranslate
	return []string{
		"en", "es", "fr", "de", "it", "pt", "ru", "zh", "ja", "ko",
		"ar", "hi", "tr", "pl", "nl", "sv", "da", "fi", "no", "cs",
		"ro", "hu", "bg", "hr", "sk", "sl", "et", "lv", "lt", "el",
	}, nil
}

// Close closes the Python subprocess.
func (pt *PythonTranslator) Close() error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.process != nil {
		if pt.stdin != nil {
			pt.stdin.Close()
		}
		if pt.stdout != nil {
			pt.stdout.Close()
		}
		if err := pt.process.Process.Kill(); err != nil {
			return err
		}
		pt.process.Wait()
		pt.initialized = false
	}
	return nil
}

