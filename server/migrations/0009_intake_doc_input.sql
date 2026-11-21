-- 0009: allow document intake (docx/txt/md) in addition to text/image/video.
ALTER TABLE admin_intake_jobs DROP CONSTRAINT IF EXISTS admin_intake_jobs_input_type_check;
ALTER TABLE admin_intake_jobs ADD CONSTRAINT admin_intake_jobs_input_type_check
    CHECK (input_type IN ('text', 'image', 'video', 'doc'));
