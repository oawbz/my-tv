package com.lizongying.mytv

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.GestureDetector
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
import android.view.WindowManager
import android.widget.Toast
import androidx.fragment.app.FragmentActivity
import com.lizongying.mytv.models.TVViewModel


class MainActivity : FragmentActivity() {

    private val readyFragments = mutableSetOf<String>()
    private var playbackStarted = false
    private lateinit var playerFragment: PlayerFragment
    private lateinit var mainFragment: MainFragment
    private lateinit var infoFragment: InfoFragment
    private lateinit var settingFragment: SettingFragment

    private var doubleBackToExitPressedOnce = false

    private lateinit var gestureDetector: GestureDetector

    private val handler = Handler()
    private val delayHideMain: Long = 10000
    private val delayHideSetting: Long = 10000

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate")
        super.onCreate(savedInstanceState)

        val restored = supportFragmentManager.fragments
        playerFragment = restored.filterIsInstance<PlayerFragment>().firstOrNull() ?: PlayerFragment()
        mainFragment = restored.filterIsInstance<MainFragment>().firstOrNull() ?: MainFragment()
        infoFragment = restored.filterIsInstance<InfoFragment>().firstOrNull() ?: InfoFragment()
        settingFragment = restored.filterIsInstance<SettingFragment>().firstOrNull() ?: SettingFragment()

        setContentView(R.layout.activity_main)

        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        window.addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN)
        window.decorView.systemUiVisibility = SYSTEM_UI_FLAG_HIDE_NAVIGATION

        if (savedInstanceState == null) {
            supportFragmentManager.beginTransaction()
                .add(R.id.main_browse_fragment, playerFragment)
                .add(R.id.main_browse_fragment, infoFragment)
                .add(R.id.main_browse_fragment, mainFragment)
                .hide(mainFragment)
                .commit()
        }
        gestureDetector = GestureDetector(this, GestureListener())

    }

    fun showInfoFragment(tvViewModel: TVViewModel) {
        infoFragment.show(tvViewModel)
    }

    fun reloadChannels() = mainFragment.reloadChannels()

    fun play(tvViewModel: TVViewModel) {
        playerFragment.play(tvViewModel)
        mainFragment.view?.requestFocus()
    }

    fun showChannelSwitching() {
        playerFragment.showSwitching()
    }

    fun prev() {
        mainFragment.prev()
    }

    fun next() {
        mainFragment.next()
    }

    fun switchMainFragment() {
        val transaction = supportFragmentManager.beginTransaction()

        if (mainFragment.isHidden) {
            transaction.show(mainFragment)
            mainActive()
        } else {
            transaction.hide(mainFragment)
        }

        transaction.commit()
    }

    fun mainActive() {
        handler.removeCallbacks(hideMain)
        handler.postDelayed(hideMain, delayHideMain)
    }

    fun settingDelayHide() {
        handler.removeCallbacks(hideSetting)
        handler.postDelayed(hideSetting, delayHideSetting)
    }

    private val hideMain = Runnable {
        if (!mainFragment.isHidden) {
            supportFragmentManager.beginTransaction().hide(mainFragment).commit()
        }
    }

    private fun hideMainFragment() {
        if (!mainFragment.isHidden) {
            supportFragmentManager.beginTransaction()
                .hide(mainFragment)
                .commit()
        }
    }

    fun fragmentReady(tag: String) {
        readyFragments.add(tag)
        Log.i(TAG, "ready $tag")
        if (readyFragments.containsAll(REQUIRED_FRAGMENTS) && !playbackStarted) {
            playbackStarted = true
            mainFragment.fragmentReady()
        }
    }

    fun fragmentUnavailable(tag: String) {
        readyFragments.remove(tag)
        playbackStarted = false
    }

    override fun onTouchEvent(event: MotionEvent?): Boolean {
        if (event != null) {
            gestureDetector.onTouchEvent(event)
        }
        return super.onTouchEvent(event)
    }

    private inner class GestureListener : GestureDetector.SimpleOnGestureListener() {

        override fun onSingleTapConfirmed(e: MotionEvent): Boolean {
            switchMainFragment()
            return true
        }

        override fun onDoubleTap(e: MotionEvent): Boolean {
            showSetting()
            return true
        }

        override fun onFling(
            e1: MotionEvent?,
            e2: MotionEvent,
            velocityX: Float,
            velocityY: Float
        ): Boolean {
            if (velocityY > 0) {
                if (mainFragment.isHidden) {
                    prev()
                }
            }
            if (velocityY < 0) {
                if (mainFragment.isHidden) {
                    next()
                }
            }
            return super.onFling(e1, e2, velocityX, velocityY)
        }
    }

    private fun showSetting() {
        if (!mainFragment.isHidden) {
            return
        }

        Log.i(TAG, "settingFragment ${settingFragment.isVisible}")
        if (!settingFragment.isVisible) {
            settingFragment.show(supportFragmentManager, "setting")
            settingDelayHide()
        } else {
            handler.removeCallbacks(hideSetting)
            settingFragment.dismiss()
        }
    }

    private val hideSetting = Runnable {
        if (settingFragment.isVisible) {
            settingFragment.dismiss()
        }
    }

    private fun channelUp() {
        if (mainFragment.isHidden) {
            if (SP.channelReversal) {
                next()
                return
            }
            prev()
        }
    }

    private fun channelDown() {
        if (mainFragment.isHidden) {
            if (SP.channelReversal) {
                prev()
                return
            }
            next()
        }
    }

    private fun back() {
        if (!mainFragment.isHidden) {
            hideMainFragment()
            return
        }

        if (doubleBackToExitPressedOnce) {
            super.onBackPressed()
            return
        }

        doubleBackToExitPressedOnce = true
        Toast.makeText(this, "再按一次退出", Toast.LENGTH_SHORT).show()

        Handler(Looper.getMainLooper()).postDelayed({
            doubleBackToExitPressedOnce = false
        }, 2000)
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent?): Boolean {
        Log.i(TAG, "keyCode $keyCode, event $event")
        when (keyCode) {
            KeyEvent.KEYCODE_ESCAPE -> {
                back()
                return true
            }

            KeyEvent.KEYCODE_BACK -> {
                back()
                return true
            }

            KeyEvent.KEYCODE_BOOKMARK -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_UNKNOWN -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_HELP -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_SETTINGS -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_MENU -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_ENTER -> {
                switchMainFragment()
            }

            KeyEvent.KEYCODE_DPAD_CENTER -> {
                switchMainFragment()
            }

            KeyEvent.KEYCODE_DPAD_UP -> {
                channelUp()
            }

            KeyEvent.KEYCODE_CHANNEL_UP -> {
                channelUp()
            }

            KeyEvent.KEYCODE_DPAD_DOWN -> {
                channelDown()
            }

            KeyEvent.KEYCODE_CHANNEL_DOWN -> {
                channelDown()
            }

            KeyEvent.KEYCODE_DPAD_LEFT -> {
                if (!mainFragment.isVisible && !settingFragment.isVisible) {
                    switchMainFragment()
                    return true
                }
            }

            KeyEvent.KEYCODE_DPAD_RIGHT -> {
                if (!mainFragment.isVisible && !settingFragment.isVisible) {
                    showSetting()
                    return true
                }
            }
        }

        return super.onKeyDown(keyCode, event)
    }

    override fun onStart() {
        Log.i(TAG, "onStart")
        super.onStart()
    }

    override fun onResume() {
        Log.i(TAG, "onResume")
        super.onResume()
        if (!mainFragment.isHidden) {
            handler.postDelayed(hideMain, delayHideMain)
        }
    }

    override fun onPause() {
        Log.i(TAG, "onPause")
        super.onPause()
        handler.removeCallbacks(hideMain)
        handler.removeCallbacks(hideSetting)
    }

    private companion object {
        const val TAG = "MainActivity"
        val REQUIRED_FRAGMENTS = setOf(
            "PlayerFragment", "MainFragment", "InfoFragment"
        )
    }
}
