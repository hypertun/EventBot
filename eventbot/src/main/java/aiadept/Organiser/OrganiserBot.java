package aiadept.Organiser;

import org.telegram.telegrambots.client.okhttp.OkHttpTelegramClient;
import org.telegram.telegrambots.longpolling.util.LongPollingSingleThreadUpdateConsumer;
import org.telegram.telegrambots.meta.api.methods.send.SendMessage;
import org.telegram.telegrambots.meta.api.objects.PhotoSize;
import org.telegram.telegrambots.meta.api.objects.Update;
import org.telegram.telegrambots.meta.exceptions.TelegramApiException;
import org.telegram.telegrambots.meta.generics.TelegramClient;

import aiadept.Organiser.Model.Event;
import aiadept.Organiser.Repo.Firebase;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class OrganiserBot implements LongPollingSingleThreadUpdateConsumer {
    private final TelegramClient telegramClient;
    private static final Logger logger = LoggerFactory.getLogger(OrganiserBot.class);

    private final String setEventNameMsg = "Add in Name of New Event";
    private final String sendEDMMsg = "Please send EDM for the event.";
    private final String eventCreatedMsg = "Event created successfully!";

    // Store event data temporarily. Key: chat_id, Value: EventData
    private Map<Long, Event> eventsMap = new HashMap<>();

    Firebase firebase = new Firebase("/Users/ivanyeo/Desktop/Projects/EventBot/eventbot/saKey/aiadept-firebase-adminsdk-fbsvc-6f76afe22e.json");

    public OrganiserBot(String botToken) {
        telegramClient = new OkHttpTelegramClient(botToken);
    }

    @Override
    public void consume(Update update) {
        if (update.hasMessage()) {
            long chat_id = update.getMessage().getChatId();

            // Check if user is in the process of adding an event
            if (eventsMap.containsKey(chat_id)) {
                Event currentEventData = eventsMap.get(chat_id);

                // Check if event name is already set
                if (currentEventData.getEventName() == null) {
                    if (update.getMessage().hasText()) {
                        String eventName = update.getMessage().getText();
                        currentEventData.setEventName(eventName);
                        sendMessage(chat_id, sendEDMMsg);
                    } else {
                        sendMessage(chat_id, "Please enter a valid event name.");
                    }
                } else if (currentEventData.getEDMFileId() == null) {
                    // Event name is set, now expect a picture
                    if (update.getMessage().hasPhoto()) {
                        List<PhotoSize> photos = update.getMessage().getPhoto();
                        // Get the largest photo
                        PhotoSize photo = photos.stream().max(Comparator.comparing(PhotoSize::getFileSize)).orElse(null);
                        if (photo != null) {
                            String fileId = photo.getFileId();
                            currentEventData.setEDMFileId(fileId);
                            sendMessage(chat_id, eventCreatedMsg);
                            String documentId = firebase.addEvent(eventsMap.get(chat_id));
                            System.out.println("Added event with ID: " + documentId);
                            eventsMap.remove(chat_id); // Remove the event data after completion
                        }
                    } else {
                        sendMessage(chat_id, "Please send the EDM for the event.");
                    }
                }
            } else {
                // Not in the process of adding an event, check for commands
                if (update.getMessage().hasText()) {
                    String message_text = update.getMessage().getText();
                    if (message_text.equals("/addEvent")) {
                        // Start the event creation process
                        eventsMap.put(chat_id, new Event());
                        sendMessage(chat_id, setEventNameMsg);
                    }
                }
            }
        }

        logger.info(eventsMap.toString());
    }

    private void sendMessage(long chat_id, String text) {
        SendMessage message = SendMessage.builder().chatId(chat_id).text(text).build();
        try {
            telegramClient.execute(message);
        } catch (TelegramApiException e) {
            logger.error("Error sending message", e);
        }
    }
}
