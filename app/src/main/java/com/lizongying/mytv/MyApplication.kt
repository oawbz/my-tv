package com.lizongying.mytv

import androidx.multidex.MultiDexApplication

class MyApplication : MultiDexApplication() {
    override fun onCreate() {
        super.onCreate()
        SP.init(this)
        LogoLoader.register(this)

        // Remove channel data saved by earlier versions; startup now uses remote JSON only.
        getFileStreamPath("channels.json").delete()

    }
}
