package com.lizongying.mytv

import android.content.Context
import android.content.SharedPreferences

object SP {
    // Name of the sp file TODO Should use a meaningful name and do migrations
    private const val SP_FILE_NAME = "MainActivity"

    // If Change channel with up and down in reversed order or not
    private const val KEY_CHANNEL_REVERSAL = "channel_reversal"
    private const val KEY_CHANNEL_NUMBER_INPUT = "channel_number_input"
    private const val KEY_MOBILE_MODE = "mobile_mode"
    private const val KEY_MOBILE_SWIPE_CHANNEL = "mobile_swipe_channel"

    // If start app on device boot or not
    private const val KEY_BOOT_STARTUP = "boot_startup"
    private const val KEY_CHANNELS_URL = "channels_url"

    // Position in list of the selected channel item
    private const val KEY_POSITION = "position"
    private const val KEY_SELECTED_CHANNEL = "selected_channel"

    private lateinit var sp: SharedPreferences

    /**
     * The method must be invoked as early as possible(At least before using the keys)
     */
    fun init(context: Context) {
        sp = context.getSharedPreferences(SP_FILE_NAME, Context.MODE_PRIVATE)
    }

    var channelReversal: Boolean
        get() = sp.getBoolean(KEY_CHANNEL_REVERSAL, false)
        set(value) = sp.edit().putBoolean(KEY_CHANNEL_REVERSAL, value).apply()

    var channelNumberInput: Boolean
        get() = sp.getBoolean(KEY_CHANNEL_NUMBER_INPUT, false)
        set(value) = sp.edit().putBoolean(KEY_CHANNEL_NUMBER_INPUT, value).apply()

    var mobileMode: Boolean
        get() = sp.getBoolean(KEY_MOBILE_MODE, false)
        set(value) = sp.edit().putBoolean(KEY_MOBILE_MODE, value).apply()

    var mobileSwipeChannel: Boolean
        get() = sp.getBoolean(KEY_MOBILE_SWIPE_CHANNEL, true)
        set(value) = sp.edit().putBoolean(KEY_MOBILE_SWIPE_CHANNEL, value).apply()

    var bootStartup: Boolean
        get() = sp.getBoolean(KEY_BOOT_STARTUP, false)
        set(value) = sp.edit().putBoolean(KEY_BOOT_STARTUP, value).apply()

    var channelsUrl: String
        get() = sp.getString(KEY_CHANNELS_URL, "https://awbz.cn/tv/channels.json").orEmpty()
        set(value) = sp.edit().putString(KEY_CHANNELS_URL, value).apply()

    var itemPosition: Int
        get() = sp.getInt(KEY_POSITION, 0)
        set(value) = sp.edit().putInt(KEY_POSITION, value).apply()

    var selectedChannel: String
        get() = sp.getString(KEY_SELECTED_CHANNEL, "").orEmpty()
        set(value) = sp.edit().putString(KEY_SELECTED_CHANNEL, value).apply()

}
