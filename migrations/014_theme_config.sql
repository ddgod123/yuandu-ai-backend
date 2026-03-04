-- Add config column to themes table to store UI styles
ALTER TABLE taxonomy.themes ADD COLUMN IF NOT EXISTS config JSONB DEFAULT '{}';
COMMENT ON COLUMN taxonomy.themes.config IS 'UI configuration for the theme (colors, fonts, layout, etc.)';
