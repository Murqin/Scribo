import os
import logging
from telegram import Update
from telegram.ext import ApplicationBuilder, CommandHandler, MessageHandler, CallbackQueryHandler, filters, ContextTypes
from dotenv import load_dotenv

load_dotenv()

from api.index import process_voice, get_mode_keyboard, ALLOWED_USER_ID, TELEGRAM_TOKEN

logging.basicConfig(format='%(asctime)s - %(name)s - %(levelname)s - %(message)s', level=logging.INFO)
logger = logging.getLogger(__name__)

async def start_handler(update: Update, context: ContextTypes.DEFAULT_TYPE):
    user = update.effective_user
    if not user or str(user.id) != ALLOWED_USER_ID:
        return
    await update.message.reply_text("🎙️ Scribo Bot hazır! Bir ses kaydı gönderin.")

async def voice_handler(update: Update, context: ContextTypes.DEFAULT_TYPE):
    user = update.effective_user
    if not user or str(user.id) != ALLOWED_USER_ID:
        return
    
    voice = update.message.voice
    if not voice:
        return

    duration = voice.duration
    warning_text = ""
    if duration > 90:
        warning_text = f"\n\n⚠️ <b>Uyarı:</b> Ses kaydı uzun ({duration} sn)."

    await update.message.reply_html(
        text=f"🚀 Ses kaydı alındı! Seçiniz:{warning_text}",
        reply_markup=get_mode_keyboard()
    )

async def callback_handler(update: Update, context: ContextTypes.DEFAULT_TYPE):
    query = update.callback_query
    user = update.effective_user
    if not user or str(user.id) != ALLOWED_USER_ID:
        await query.answer("Yetkisiz kullanıcı.", show_alert=True)
        return

    await query.answer()
    callback_data = query.data

    if callback_data == "cancel_paid":
        await query.edit_message_text(text="❌ İşlem iptal edildi.")
        return

    force_provider = None
    if callback_data.startswith("paid:"):
        mode = callback_data.split(":")[1]
        force_provider = "openrouter"
    else:
        mode = callback_data

    voice_msg = query.message.reply_to_message
    if voice_msg and voice_msg.voice:
        await process_voice(
            chat_id=query.message.chat.id,
            file_id=voice_msg.voice.file_id,
            mode=mode,
            message_id=query.message.message_id,
            force_provider=force_provider
        )
    else:
        await query.edit_message_text(text="❌ Kaynak ses dosyası bulunamadı.")

def main():
    if not TELEGRAM_TOKEN:
        raise ValueError("TELEGRAM_TOKEN ortam değişkeni tanımlı değil!")
    
    app = ApplicationBuilder().token(TELEGRAM_TOKEN).build()
    
    app.add_handler(CommandHandler("start", start_handler))
    app.add_handler(MessageHandler(filters.VOICE, voice_handler))
    app.add_handler(CallbackQueryHandler(callback_handler))

    logger.info("🤖 Scribo Bot Oracle Cloud VPS üzerinde Polling modunda başlatılıyor...")
    app.run_polling(drop_pending_updates=True)

if __name__ == "__main__":
    main()
