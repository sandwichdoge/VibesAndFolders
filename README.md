# VibesAndFolders

VibesAndFolders is a desktop tool that uses AI to organize your files based on plain English instructions. Instead of manually sorting hundreds of files, simply describe how you want them arranged, and the application handles the rest.

It features a built-in preview mode and file count verification to ensure no files are lost during the process.

### Basic Usage:

Launch the application.

Go to Settings > Configure to enter your AI Provider details (Endpoint and API Key). Compatible with locally hosted models.

Click Browse to select the messy folder you want to clean up.

Type your instructions in the text box (e.g., "Move all images into a Photos folder and documents into a Docs folder").

Click Analyze to see a preview of the changes.

If the preview looks correct, click Execute to apply the changes.

Download: https://github.com/sandwichdoge/vibesandfolders/releases/

### How to build and run from source:
```
make setup
make build
make run
```