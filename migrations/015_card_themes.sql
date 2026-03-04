-- Create card_themes table for UI templates
CREATE TABLE IF NOT EXISTS archive.card_themes (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    slug VARCHAR(64) UNIQUE NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    is_system BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE archive.card_themes IS 'Stores UI design templates for XHS cards';

-- Insert the current Romantic/Couple design as the first template
INSERT INTO archive.card_themes (name, slug, config, is_system) 
VALUES (
    '情侣浪漫主题', 
    'romantic-couple', 
    '{
        "bgColor": "#fff5f5",
        "textColor": "#1e293b",
        "accentColor": "#fb7185",
        "fontFamily": "serif",
        "gridStyle": "sticker",
        "showFeatured": true
    }', 
    true
) ON CONFLICT (slug) DO NOTHING;

INSERT INTO archive.card_themes (name, slug, config, is_system) 
VALUES (
    '简约极简主题', 
    'minimal-classic', 
    '{
        "bgColor": "#ffffff",
        "textColor": "#000000",
        "accentColor": "#10b981",
        "fontFamily": "sans",
        "gridStyle": "clean",
        "showFeatured": false
    }', 
    true
) ON CONFLICT (slug) DO NOTHING;
