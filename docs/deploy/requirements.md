# Deployment requirements for document generation

To generate PDF, DOCX, and XLSX documents reliably on Linux servers, ensure the following:

- **Filesystem permissions**
  - The configured `files.root_dir` (default: `./files`) must be writable by the application user.
  - Subdirectories `pdf`, `docx`, and `excel` will be created automatically on startup with `0755` permissions.

- **Templates and fonts**
  - Ship template directories alongside the binary:
    - DOCX templates: `assets/templates/docx`
    - XLSX templates: `assets/templates/xlsx`
    - TXT (PDF) templates: `assets/templates/txt`
  - Ensure the font used by the PDF generator (`assets/fonts/DejaVuSans.ttf`) is present.

- **LibreOffice (optional, for DOCX → PDF conversion)**
  - Install the `libreoffice` package if `libreoffice.enable` is set to `true` in `config.yaml`.
  - Configure `libreoffice.binary` if the `soffice`/`libreoffice` binary is not on `PATH`.
  - The service runs LibreOffice in headless mode: `--headless --convert-to pdf --outdir <pdfDir> <docxFile>`.

- **Environment**
  - The server should have locale and fonts configured so generated PDFs render correctly.
  - Avoid Windows-style paths; all paths are normalized with forward slashes.
