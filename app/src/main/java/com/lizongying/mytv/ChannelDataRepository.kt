package com.lizongying.mytv

import android.net.Uri
import android.util.Log
import com.google.gson.Gson
import com.google.gson.JsonParseException
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.util.concurrent.TimeUnit

object ChannelDataRepository {
    private const val TAG = "ChannelDataRepository"
    private const val TIMEOUT_SECONDS = 5L
    private const val MAX_CONFIG_BYTES = 1024 * 1024

    private val gson = Gson()
    private val client = OkHttpClient.Builder()
        .connectTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
        .readTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
        .callTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
        .build()

    fun loadChannels(url: String = SP.channelsUrl): Map<String, List<TV>> {
        val channels = parseChannels(fetchRemoteJson(url))
        Log.i(TAG, "Loaded ${channels.values.sumOf { it.size }} channels from remote JSON")
        return channels
    }

    private fun fetchRemoteJson(url: String): String {
        if (!isHttpUrl(url)) throw IOException("Channel URL must be a valid HTTP or HTTPS URL")
        val request = Request.Builder().url(url).get().build()
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) {
                throw IOException("Channel request failed with HTTP ${response.code}")
            }
            val body = response.body ?: throw IOException("Channel response body is empty")
            if (body.contentLength() > MAX_CONFIG_BYTES) {
                throw IOException("Channel response is too large")
            }
            val output = ByteArrayOutputStream()
            body.byteStream().use { input ->
                val buffer = ByteArray(8192)
                while (true) {
                    val count = input.read(buffer)
                    if (count < 0) break
                    if (output.size() + count > MAX_CONFIG_BYTES) {
                        throw IOException("Channel response is too large")
                    }
                    output.write(buffer, 0, count)
                }
            }
            return output.toString(Charsets.UTF_8.name())
        }
    }

    private fun parseChannels(json: String): Map<String, List<TV>> {
        val config = try {
            gson.fromJson(json, RemoteChannels::class.java)
        } catch (e: JsonParseException) {
            throw IOException("Invalid channel JSON", e)
        } ?: throw IOException("Channel JSON is empty")

        if (config.version != 1) {
            throw IOException("Unsupported channel config version: ${config.version}")
        }
        val entries = config.channels
        if (entries.isNullOrEmpty()) {
            throw IOException("Channel list is empty")
        }

        val grouped = linkedMapOf<String, MutableList<TV>>()
        for (entry in entries) {
            if (entry == null) {
                Log.w(TAG, "Skipping null channel entry")
                continue
            }
            val name = entry.name?.trim().takeUnless { it.isNullOrEmpty() }
            if (name == null) {
                Log.w(TAG, "Skipping channel without a name")
                continue
            }
            val group = entry.group?.trim().takeUnless { it.isNullOrEmpty() } ?: "其他"
            val urls = entry.urls?.mapNotNull { it?.trim()?.takeIf(::isHttpUrl) }
                ?: emptyList()
            if (urls.isEmpty()) {
                Log.w(TAG, "Skipping channel $name without a valid playback URL")
                continue
            }

            val tv = TV(
                title = name,
                videoUrl = urls,
                channel = group,
                logo = entry.logo?.trim()?.takeIf(::isHttpUrl).orEmpty(),
            )
            grouped.getOrPut(group) { mutableListOf() }.add(tv)
        }

        if (grouped.isEmpty()) {
            throw IOException("Channel list has no usable entries")
        }
        return grouped
    }

    private fun isHttpUrl(value: String): Boolean {
        val uri = Uri.parse(value)
        return (uri.scheme == "https" || uri.scheme == "http") && !uri.host.isNullOrBlank()
    }

    private data class RemoteChannels(
        val version: Int = 0,
        val channels: List<RemoteChannel?>? = null,
    )

    private data class RemoteChannel(
        val name: String? = null,
        val group: String? = null,
        val logo: String? = null,
        val urls: List<String?>? = null,
    )
}
