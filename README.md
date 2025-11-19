# VibesAndFolders

VibesAndFolders is a desktop tool that uses AI to organize your files based on plain English instructions. Instead of manually sorting hundreds of files, simply describe how you want them arranged, and the application handles the rest.

For safety, it can only move or rename a file/folder, never delete. It will never create a new file, only folders.

All decisions are entirely based on filenames, it does not read the file content.

<img width="1030" height="901" alt="image" src="https://github.com/user-attachments/assets/206efee7-5dcf-4184-8778-3ed904a3abdd" />

### Basic Usage:

- Launch the application.
- Go to Settings > Configure to enter your AI Provider details (Endpoint and API Key). Compatible with locally hosted models.
- Click Browse to select the messy folder you want to clean up.
- Type your instructions in the text box (e.g., "Move all images into a Photos folder and documents into a Docs folder").
- Click Analyze to see a preview of the changes.
- If the preview looks correct, click Execute to apply the changes.

**Downloads (Mac, Windows, Linux):** https://github.com/sandwichdoge/vibesandfolders/releases/

### How to build and run from source:
```
make setup
make run
```
