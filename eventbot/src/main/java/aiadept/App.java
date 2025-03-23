package aiadept;

import org.telegram.telegrambots.longpolling.TelegramBotsLongPollingApplication;

import aiadept.Organiser.OrganiserBot;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;


public class App 
{
    private static final Logger logger = LoggerFactory.getLogger(App.class);
    public static void main(String[] args) {
        String botToken = "7510313727:AAHj81Bx4Iu32B6wFqpM4i28X_Z20ZEHryA";
        try (TelegramBotsLongPollingApplication botsApplication = new TelegramBotsLongPollingApplication()) {
            botsApplication.registerBot(botToken, new OrganiserBot(botToken));
                        logger.info("Bot registered successfully");

            Thread.currentThread().join();
        } catch (Exception e) {
            logger.error("Error registering bot", e);
    }

    }
}
