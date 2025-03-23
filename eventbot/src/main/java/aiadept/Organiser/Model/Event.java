package aiadept.Organiser.Model;

import java.util.List;

public class Event {
    private String eventName;
    private String photoFileId;
    private String documentID;
    private List<EventDetail> details;

    public String getDocumentID() {
        return documentID;
    }

    public void setDocumentID(String id) {
        this.documentID = id;
    }

    public String getEventName() {
        return eventName;
    }

    public void setEventName(String eventName) {
        this.eventName = eventName;
    }

    public String getEDMFileId() {
        return photoFileId;
    }

    public void setEDMFileId(String photoFileId) {
        this.photoFileId = photoFileId;
    }

    public List<EventDetail> getDetails() {
        return details;
    }

    public void setDetails(List<EventDetail> details) {
        this.details = details;
    }

    public void addDetail(EventDetail detail) {
        this.details.add(detail);
    }

    public static class EventDetail {
        private String timing;
        private String name;
        private String description;

        public EventDetail(String timing, String name, String description) {
            this.timing = timing;
            this.name = name;
            this.description = description;
        }

        public String getTiming() {
            return timing;
        }

        public void setTiming(String timing) {
            this.timing = timing;
        }

        public String getName() {
            return name;
        }

        public void setName(String name) {
            this.name = name;
        }

        public String getDescription() {
            return description;
        }

        public void setDescription(String description) {
            this.description = description;
        }
    }
}