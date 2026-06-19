ALTER TABLE public_document_signatures
ADD COLUMN IF NOT EXISTS signature_image_path TEXT;
