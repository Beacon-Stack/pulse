-- name: GetSharedMediaHandling :one
SELECT * FROM shared_media_handling WHERE id = 1;

-- name: UpdateSharedMediaHandling :one
UPDATE shared_media_handling SET
    colon_replacement     = $1,
    import_extra_files    = $2,
    extra_file_extensions = $3,
    rename_files          = $4,
    updated_at            = $5
WHERE id = 1
RETURNING *;
