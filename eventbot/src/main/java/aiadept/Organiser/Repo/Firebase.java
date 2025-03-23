package aiadept.Organiser.Repo;

import aiadept.Organiser.Model.Event;
import com.google.api.core.ApiFuture;
import com.google.auth.oauth2.GoogleCredentials;
import com.google.cloud.firestore.*;
import com.google.firebase.FirebaseApp;
import com.google.firebase.FirebaseOptions;
import com.google.firebase.cloud.FirestoreClient;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ExecutionException;

public class Firebase {

    private static final Logger logger = LoggerFactory.getLogger(Firebase.class);
    private static final String COLLECTION_NAME = "events";
    private Firestore db;

    public Firebase(String serviceAccountKeyPath) {
        try {
            initializeFirebase(serviceAccountKeyPath);
        } catch (IOException e) {
            logger.error("Error initializing Firebase", e);
            throw new RuntimeException("Failed to initialize Firebase", e);
        }
    }

    private void initializeFirebase(String serviceAccountKeyPath) throws IOException {
        InputStream serviceAccount = new FileInputStream(serviceAccountKeyPath);
        GoogleCredentials credentials = GoogleCredentials.fromStream(serviceAccount);
        FirebaseOptions options = FirebaseOptions.builder()
                .setCredentials(credentials)
                .build();

        if (FirebaseApp.getApps().isEmpty()) {
            FirebaseApp.initializeApp(options);
        }
        db = FirestoreClient.getFirestore();
        logger.info("Firebase initialized successfully");
    }

    // Create (Add)
    public String addEvent(Event event) {
        try {
            DocumentReference docRef = db.collection(COLLECTION_NAME).document();
            String documentId = docRef.getId();
            event.setDocumentID(documentId);
            ApiFuture<WriteResult> result = docRef.set(convertToFirestoreData(event));
            logger.info("Event added with ID: " + documentId + " at " + result.get().getUpdateTime());
            return documentId;
        } catch (InterruptedException | ExecutionException e) {
            logger.error("Error adding event", e);
            return null;
        }
    }

    // Read (Get)
    public Event getEvent(String documentId) {
        try {
            DocumentReference docRef = db.collection(COLLECTION_NAME).document(documentId);
            ApiFuture<DocumentSnapshot> future = docRef.get();
            DocumentSnapshot document = future.get();
            if (document.exists()) {
                logger.info("Event found: " + document.getData());
                return convertToEvent(document);
            } else {
                logger.warn("No such document: " + documentId);
                return null;
            }
        } catch (InterruptedException | ExecutionException e) {
            logger.error("Error getting event", e);
            return null;
        }
    }

    // Update
    public boolean updateEvent(Event event) {
        try {
            if (event.getDocumentID() == "") {
                logger.error("Cannot update event without document ID");
                return false;
            }
            DocumentReference docRef = db.collection(COLLECTION_NAME).document(event.getDocumentID());
            ApiFuture<WriteResult> result = docRef.set(convertToFirestoreData(event));
            logger.info("Event updated at: " + result.get().getUpdateTime());
            return true;
        } catch (InterruptedException | ExecutionException e) {
            logger.error("Error updating event", e);
            return false;
        }
    }

    // Delete
    public boolean deleteEvent(String documentId) {
        try {
            ApiFuture<WriteResult> writeResult = db.collection(COLLECTION_NAME).document(documentId).delete();
            logger.info("Event deleted at: " + writeResult.get().getUpdateTime());
            return true;
        } catch (InterruptedException | ExecutionException e) {
            logger.error("Error deleting event", e);
            return false;
        }
    }

    // Helper methods to convert between Event object and Firestore data
    private Map<String, Object> convertToFirestoreData(Event event) {
        Map<String, Object> data = new HashMap<>();
        data.put("eventName", event.getEventName());
        data.put("photoFileId", event.getEDMFileId());

        List<Map<String, Object>> detailsData = new ArrayList<>();
        if (event.getDetails() != null) {
            for (Event.EventDetail detail : event.getDetails()) {
                Map<String, Object> detailData = new HashMap<>();
                detailData.put("timing", detail.getTiming());
                detailData.put("name", detail.getName());
                detailData.put("description", detail.getDescription());
                detailsData.add(detailData);
            }
        }
        data.put("details", detailsData);
        return data;
    }

    private Event convertToEvent(DocumentSnapshot document) {
        Event event = new Event();
        event.setDocumentID(document.getId());
        event.setEventName(document.getString("eventName"));
        event.setEDMFileId(document.getString("photoFileId"));

        List<Map<String, Object>> detailsData = (List<Map<String, Object>>) document.get("details");
        if (detailsData != null) {
            List<Event.EventDetail> details = new ArrayList<>();
            for (Map<String, Object> detailData : detailsData) {
                String timing = (String) detailData.get("timing");
                String name = (String) detailData.get("name");
                String description = (String) detailData.get("description");
                Event.EventDetail detail = new Event.EventDetail(timing, name, description);
                details.add(detail);
            }
            event.setDetails(details);
        }
        return event;
    }
}
