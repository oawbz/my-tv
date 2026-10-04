package com.lizongying.mytv

import android.content.Context
import android.graphics.BitmapFactory
import android.widget.ImageView
import com.bumptech.glide.Glide
import com.bumptech.glide.Priority
import com.bumptech.glide.load.DataSource
import com.bumptech.glide.load.Options
import com.bumptech.glide.load.data.DataFetcher
import com.bumptech.glide.load.engine.DiskCacheStrategy
import com.bumptech.glide.load.model.ModelLoader
import com.bumptech.glide.load.model.ModelLoaderFactory
import com.bumptech.glide.load.model.MultiModelLoaderFactory
import com.bumptech.glide.signature.ObjectKey
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.nio.ByteBuffer
import java.util.concurrent.TimeUnit

/** Validate downloaded bytes before Glide can invoke any native image decoder. */
object LogoLoader {
    private const val MAX_BYTES = 2 * 1024 * 1024
    private val client = OkHttpClient.Builder()
        .connectTimeout(5, TimeUnit.SECONDS).readTimeout(5, TimeUnit.SECONDS)
        .callTimeout(5, TimeUnit.SECONDS).build()
    private data class Logo(val url: String)

    fun register(context: Context) {
        Glide.get(context).registry.prepend(Logo::class.java, ByteBuffer::class.java, Factory())
    }

    fun load(image: ImageView, url: String) {
        // Clear both the previous drawable and its in-flight fetch on every channel change.
        Glide.with(image).clear(image)
        Glide.with(image).asBitmap().load(if (url.isBlank()) null else Logo(url))
            .diskCacheStrategy(DiskCacheStrategy.NONE)
            .placeholder(R.drawable.logo).error(R.drawable.logo).fallback(R.drawable.logo)
            .override(300, 101).centerInside().into(image)
    }

    fun clear(image: ImageView) = Glide.with(image).clear(image)

    private class Factory : ModelLoaderFactory<Logo, ByteBuffer> {
        override fun build(multiFactory: MultiModelLoaderFactory): ModelLoader<Logo, ByteBuffer> = Loader()
        override fun teardown() {}
    }
    private class Loader : ModelLoader<Logo, ByteBuffer> {
        override fun handles(model: Logo) = true
        override fun buildLoadData(model: Logo, width: Int, height: Int, options: Options) =
            ModelLoader.LoadData(ObjectKey("safe-logo-v1:${model.url}"), Fetcher(model.url))
    }
    private class Fetcher(private val url: String) : DataFetcher<ByteBuffer> {
        @Volatile private var cancelled = false
        @Volatile private var call: Call? = null
        override fun loadData(priority: Priority, callback: DataFetcher.DataCallback<in ByteBuffer>) {
            try {
                val request = client.newCall(Request.Builder().url(url).build())
                call = request
                if (cancelled) { request.cancel(); return }
                request.enqueue(object : Callback {
                    override fun onFailure(call: Call, e: IOException) {
                        if (!cancelled) callback.onLoadFailed(e)
                    }
                    override fun onResponse(call: Call, response: Response) {
                        try {
                            val data = response.use {
                                if (!it.isSuccessful) throw IOException("Logo HTTP ${it.code()}")
                                val body = it.body() ?: throw IOException("Empty logo")
                                if (body.contentLength() > MAX_BYTES) throw IOException("Logo too large")
                                val output = ByteArrayOutputStream()
                                body.byteStream().use { input ->
                                    val buffer = ByteArray(8192)
                                    while (!cancelled) {
                                        val count = input.read(buffer)
                                        if (count < 0) break
                                        if (output.size() + count > MAX_BYTES) throw IOException("Logo too large")
                                        output.write(buffer, 0, count)
                                    }
                                }
                                output.toByteArray()
                            }
                            if (cancelled) return
                            validate(data)
                            callback.onDataReady(ByteBuffer.wrap(data))
                        } catch (e: Exception) {
                            if (!cancelled) callback.onLoadFailed(e)
                        }
                    }
                })
            } catch (e: Exception) {
                if (!cancelled) callback.onLoadFailed(e)
            }
        }
        override fun cancel() { cancelled = true; call?.cancel() }
        override fun cleanup() { call?.cancel() }
        override fun getDataClass() = ByteBuffer::class.java
        override fun getDataSource() = DataSource.REMOTE
    }

    private fun validate(data: ByteArray) {
        val png = byteArrayOf(0x89.toByte(), 0x50, 0x4e, 0x47, 13, 10, 26, 10)
        val isPNG = data.size >= 24 && png.indices.all { data[it] == png[it] }
        val isJPEG = data.size >= 3 && data[0] == 0xff.toByte() &&
            data[1] == 0xd8.toByte() && data[2] == 0xff.toByte()
        // In particular, never pass RIFF/WEBP to the API 18 Skia decoder.
        if (!isPNG && !isJPEG) throw IOException("Unsupported logo format: expected PNG or JPEG")
        if (isPNG) {
            val header = ByteBuffer.wrap(data)
            checkDimensions(header.getInt(16), header.getInt(20))
        }
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(data, 0, data.size, bounds)
        checkDimensions(bounds.outWidth, bounds.outHeight)
    }
    private fun checkDimensions(width: Int, height: Int) {
        if (width <= 0 || height <= 0 || width > 4096 || height > 4096 ||
            width.toLong() * height > 4 * 1024 * 1024) throw IOException("Invalid or oversized logo dimensions")
    }
}
