-- If default library has items, rename it to "My Library" (preserve data)
UPDATE libraries SET name = 'My Library' WHERE id = 'default'
  AND EXISTS (SELECT 1 FROM media_items WHERE library_id = 'default');

-- If default library has no items, delete it
DELETE FROM libraries WHERE id = 'default'
  AND NOT EXISTS (SELECT 1 FROM media_items WHERE library_id = 'default');

-- Clean up orphaned permissions
DELETE FROM library_permissions WHERE library_id = 'default'
  AND NOT EXISTS (SELECT 1 FROM libraries WHERE id = 'default');
