// Mini markdown renderer — just the syntax this site needs, zero dependencies.
// Supported: #–###### headings, paragraphs, -/* and 1. lists, > quotes,
// ``` code fences, --- rules, **bold**, *italic*, `inline code`,
// [link](url), ![image](url)
// Security: all input is HTML-escaped first; only tags produced here reach
// the output. Link/image URLs pass a scheme allow-list (http, https and
// in-site paths) — javascript: and friends are rejected.

function mdEscapeHtml(s) {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function mdSafeUrl(url) {
    if (/^https?:\/\//i.test(url)) return url;
    // Protocol-relative (//host) URLs lead off-site — reject
    if (/^\/\//.test(url)) return null;
    // Anything else with a scheme (javascript:, data:, …) is rejected; the
    // rest counts as an in-site path (/x, ./x, #x and bare relative paths)
    if (/^[a-z][a-z0-9+.-]*:/i.test(url)) return null;
    return url;
}

function mdInline(text) {
    // Generated HTML fragments are stashed behind NUL placeholders so later
    // rules (emphasis in particular) can't splice into finished tags/attributes
    const tokens = [];
    const stash = (html) => {
        tokens.push(html);
        return '\u0000' + (tokens.length - 1) + '\u0000';
    };

    text = text.replace(/`([^`]+)`/g, (m, code) =>
        stash('<code>' + code + '</code>')
    );

    text = text.replace(/!\[([^\]]*)\]\(([^)\s]+)\)/g, (m, alt, url) => {
        const safe = mdSafeUrl(url);
        return safe
            ? stash('<img src="' + safe + '" alt="' + alt + '" loading="lazy">')
            : m;
    });

    text = text.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (m, label, url) => {
        const safe = mdSafeUrl(url);
        if (!safe) return m;
        const external = /^https?:\/\//i.test(safe)
            ? ' target="_blank" rel="noopener noreferrer"'
            : '';
        return stash('<a href="' + safe + '"' + external + '>' + label + '</a>');
    });

    // Emphasis content can't start or end with whitespace, so stray asterisks
    // in text like "rm *.log and *.tmp" stay literal
    text = text.replace(/\*\*([^\s*](?:[^*]*[^\s*])?)\*\*/g, '<strong>$1</strong>');
    text = text.replace(/\*([^\s*](?:[^*]*[^\s*])?)\*/g, '<em>$1</em>');

    return text.replace(/\u0000(\d+)\u0000/g, (m, i) => tokens[+i]);
}

function markdownToHtml(md) {
    // Input is fully escaped up front — ">" arrives below as "&gt;".
    // NUL bytes are stripped so they can't collide with the placeholders
    const cleaned = md.replace(/\u0000/g, '').replace(/\r\n?/g, '\n');
    const lines = mdEscapeHtml(cleaned).split('\n');
    const out = [];
    let i = 0;

    const isBlockStart = (line) =>
        /^(#{1,6}\s|```|[-*]\s|\d+\.\s|&gt;)/.test(line) ||
        /^(---+|\*\*\*+)\s*$/.test(line);

    while (i < lines.length) {
        const line = lines[i];

        if (!line.trim()) { i++; continue; }

        const fence = line.match(/^(`{3,})/);
        if (fence) {
            // Closing needs at least as many backticks as the opening,
            // so a ```` block can contain a ``` example
            const closeRe = new RegExp('^`{' + fence[1].length + ',}\\s*$');
            const buf = [];
            i++;
            while (i < lines.length && !closeRe.test(lines[i])) {
                buf.push(lines[i]);
                i++;
            }
            i++; // skip the closing fence
            out.push('<pre><code>' + buf.join('\n') + '</code></pre>');
            continue;
        }

        const heading = line.match(/^(#{1,6})\s+(.*)$/);
        if (heading) {
            // The post title from posts.json is the page's only h1; a
            // markdown # is demoted to h2 so a second h1 can never appear
            const level = Math.max(2, heading[1].length);
            out.push('<h' + level + '>' + mdInline(heading[2]) + '</h' + level + '>');
            i++;
            continue;
        }

        if (/^(---+|\*\*\*+)\s*$/.test(line)) {
            out.push('<hr>');
            i++;
            continue;
        }

        if (/^&gt;\s?/.test(line)) {
            const buf = [];
            while (i < lines.length && /^&gt;\s?/.test(lines[i])) {
                buf.push(lines[i].replace(/^&gt;\s?/, ''));
                i++;
            }
            out.push('<blockquote><p>' + mdInline(buf.join(' ')) + '</p></blockquote>');
            continue;
        }

        const listMatch = /^[-*]\s+/.test(line) || /^\d+\.\s+/.test(line);
        if (listMatch) {
            const ordered = /^\d+\.\s+/.test(line);
            const marker = ordered ? /^\d+\.\s+/ : /^[-*]\s+/;
            const items = [];
            while (i < lines.length && marker.test(lines[i])) {
                items.push('<li>' + mdInline(lines[i].replace(marker, '')) + '</li>');
                i++;
            }
            const tag = ordered ? 'ol' : 'ul';
            out.push('<' + tag + '>' + items.join('') + '</' + tag + '>');
            continue;
        }

        // Paragraph: merge lines until a blank line or another block start
        const buf = [line];
        i++;
        while (i < lines.length && lines[i].trim() && !isBlockStart(lines[i])) {
            buf.push(lines[i]);
            i++;
        }
        out.push('<p>' + mdInline(buf.join(' ')) + '</p>');
    }

    return out.join('\n');
}
