package com.lizongying.mytv

import android.app.Application

class MyApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        SP.init(this)

        // Remove channel data saved by earlier versions; startup now uses remote JSON only.
        getFileStreamPath("channels.json").delete()

    }
}
