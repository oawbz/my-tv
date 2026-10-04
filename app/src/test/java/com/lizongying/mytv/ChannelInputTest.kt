package com.lizongying.mytv

import org.junit.Assert.*
import org.junit.Test
import java.io.IOException

class ChannelInputTest {
    @Test fun rejectsDeepUnknownFieldsBeforeGson() {
        rejected("{\"unknown\":" + "[".repeat(10000) + "0" + "]".repeat(10000) + "}")
    }

    @Test fun nestingInsideEscapedStringsDoesNotCount() {
        ChannelInput.checkJson("{\"name\":\"" + "[".repeat(100) + "\\\"quoted\\\"\",\"urls\":[]}")
        ChannelInput.checkJson("[".repeat(32) + "0" + "]".repeat(32))
        rejected("[".repeat(33) + "0" + "]".repeat(33))
    }

    @Test fun rejectsTruncatedAndOversizedJson() {
        rejected("{\"name\":\"unfinished")
        rejected("{\"channels\":[]")
        rejected("}")
        rejected(" ".repeat(1024 * 1024 + 1))
    }

    @Test fun urlsMustBeAcceptedByNetworkClient() {
        assertTrue(ChannelInput.isHttpUrl("http://192.168.1.233:2219/channels.json"))
        assertTrue(ChannelInput.isHttpUrl("https://example.com/live.m3u8?toke=a%20b"))
        listOf("", "file:///tmp/a", "http://", "http://host:99999/a",
            "http://host:abc/a", "http://host/a\n", "http://host/a b",
            "http://host/" + "a".repeat(8192)).forEach {
            assertFalse(it, ChannelInput.isHttpUrl(it))
        }
    }

    private fun rejected(json: String) {
        try {
            ChannelInput.checkJson(json)
            fail("Expected invalid input to be rejected")
        } catch (_: IOException) {}
    }
}
