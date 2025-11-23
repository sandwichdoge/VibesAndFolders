# VibesAndFolders

VibesAndFolders is a desktop tool that uses AI to organize your files based on plain English instructions. Instead of manually sorting hundreds of files, simply describe how you want them arranged, and the application handles the rest.

For safety, it can only move or rename a file/folder, never delete. It will never create a new file, only folders.

By default, all decisions are entirely based on filenames, it does not read the file content. However, you can enable **Deep Analysis** mode to leverage multimodal AI to analyze text and image files for smarter organization.

<img width="1030" height="901" alt="image" src="https://github.com/user-attachments/assets/206efee7-5dcf-4184-8778-3ed904a3abdd" />

### Basic Usage:

- Launch the application.
- Go to Settings > Configure to enter your AI Provider details (Endpoint and API Key). Compatible with locally hosted models.
- Click Browse to select the messy folder you want to clean up.
- Type your instructions in the text box (e.g., "Move all images into a Photos folder and documents into a Docs folder").
- Click Analyze to see a preview of the changes.
- If the preview looks correct, click Execute to apply the changes.

**Downloads (Mac, Windows, Linux):** https://github.com/sandwichdoge/vibesandfolders/releases/

### Deep Analysis Feature:

The Deep Analysis feature uses multimodal AI to intelligently index and analyze your files:

**What it does:**
- Indexes files in a SQLite database with AI-generated descriptions
- Analyzes text, docs, excel, files, images (via vision AI), and PDFs
- **Sends file descriptions to the AI** when organizing - the AI sees content summaries, not just filenames
- Tracks file changes using last-modified timestamps for efficient change detection
- Skips analysis of large files (>50KB for text, >5MB for images, >50MB for PDFs)

**Performance:**
- Only new or modified files are analyzed (uses file modification time for change detection)
- Large files are skipped to avoid processing overhead
- Index is stored locally in SQLite for fast access

### How to build and run from source:
```
make setup
make run
```

TODO:
Custom prompt for deep analysis indexing