package com.lizongying.mytv

import android.util.Log
import com.google.gson.Gson
import com.google.gson.JsonParseException
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

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

    suspend fun loadChannels(url: String): Map<String, List<TV>> {
        val channels = parseChannels(fetchRemoteJson(url))
        Log.i(TAG, "Loaded ${channels.values.sumOf { it.size }} channels from remote JSON")
        return channels
    }

    private suspend fun fetchRemoteJson(url: String): String {
        if (!ChannelInput.isHttpUrl(url)) throw IOException("Channel URL must be a valid HTTP or HTTPS URL")
        val request = Request.Builder().url(url).get().build()
        return suspendCancellableCoroutine { continuation ->
            val call = client.newCall(request)
            continuation.invokeOnCancellation { call.cancel() }
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    if (continuation.isActive) continuation.resumeWithException(e)
                }

                override fun onResponse(call: Call, response: Response) {
                    try {
                        val json = response.use { readResponse(it) }
                        if (continuation.isActive) continuation.resume(json)
                    } catch (e: Exception) {
                        if (continuation.isActive) continuation.resumeWithException(e)
                    }
                }
            })
        }
    }

    private fun readResponse(response: Response): String {
        if (!response.isSuccessful) {
            throw IOException("Channel request failed with HTTP ${response.code()}")
        }
        val body = response.body() ?: throw IOException("Channel response body is empty")
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

    private fun parseChannels(json: String): Map<String, List<TV>> {
        ChannelInput.checkJson(json)
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
        if (entries.size > ChannelInput.MAX_CHANNELS) throw IOException("Too many channels")

        val grouped = linkedMapOf<String, MutableList<TV>>()
        for (entry in entries) {
            if (entry == null) {
                Log.w(TAG, "Skipping null channel entry")
                continue
            }
            val name = entry.name?.trim().takeUnless { it.isNullOrEmpty() }
            if (name == null || name.length > 256) {
                Log.w(TAG, "Skipping channel without a name")
                continue
            }
            val group = entry.group?.trim().takeUnless { it.isNullOrEmpty() } ?: "其他"
            if (group.length > 128) continue
            val urls = entry.urls?.asSequence()?.mapNotNull {
                it?.trim()?.takeIf(ChannelInput::isHttpUrl)
            }?.distinct()?.take(ChannelInput.MAX_SOURCES)?.toList()
                ?: emptyList()
            if (urls.isEmpty()) {
                Log.w(TAG, "Skipping channel $name without a valid playback URL")
                continue
            }

            val tv = TV(
                title = name,
                videoUrl = urls,
                channel = group,
                logo = entry.logo?.trim()?.takeIf(ChannelInput::isHttpUrl).orEmpty(),
            )
            if (group !in grouped && grouped.size >= ChannelInput.MAX_GROUPS) {
                throw IOException("Too many channel groups")
            }
            grouped.getOrPut(group) { mutableListOf() }.add(tv)
        }

        if (grouped.isEmpty()) {
            throw IOException("Channel list has no usable entries")
        }
        return grouped
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
