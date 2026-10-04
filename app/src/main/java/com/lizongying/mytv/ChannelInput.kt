package com.lizongying.mytv

import okhttp3.HttpUrl
import java.io.IOException

internal object ChannelInput {
    const val MAX_CHANNELS = 2000
    const val MAX_GROUPS = 128
    const val MAX_SOURCES = 16

    fun isHttpUrl(value: String): Boolean =
        value.length <= 8192 && value.none { it <= ' ' || it == '\u007f' } &&
            HttpUrl.parse(value) != null

    // Gson 2.8 recurses while parsing even unknown fields. Bound nesting before parsing.
    fun checkJson(json: String) {
        if (json.length > 1024 * 1024) throw IOException("Channel JSON is too large")
        var depth = 0
        var quoted = false
        var escaped = false
        for (char in json) {
            if (quoted) {
                if (escaped) escaped = false
                else if (char == '\\') escaped = true
                else if (char == '"') quoted = false
            } else {
                when (char) {
                    '"' -> quoted = true
                    '{', '[' -> if (++depth > 32) throw IOException("Channel JSON is nested too deeply")
                    '}', ']' -> if (--depth < 0) throw IOException("Unbalanced channel JSON")
                }
            }
        }
        if (quoted || depth != 0) throw IOException("Incomplete channel JSON")
    }
}
